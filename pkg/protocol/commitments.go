package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Commitments is a subcomponent of the protocol that exposes the commitments that are managed by the protocol and that
// are either published from the network or created by an engine of the node.
type Commitments struct {
	// Set contains all non-evicted commitments that are managed by the protocol.
	reactive.Set[*Commitment]

	// Root contains the root commitment.
	Root reactive.Variable[*Commitment]

	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// cachedRequests contains Promise instances for all non-evicted commitments that were requested by the Protocol.
	// It acts as a cache and a way to address commitments generically even if they are still unsolid.
	cachedRequests *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	// Logger contains a reference to the logger that is used by this component.
	log.Logger
}

// newCommitments creates a new commitments instance for the given protocol.
func newCommitments(protocol *Protocol) *Commitments {
	c := &Commitments{
		Set:            reactive.NewSet[*Commitment](),
		Root:           reactive.NewVariable[*Commitment](),
		protocol:       protocol,
		cachedRequests: shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
	}

	shutdown := lo.Batch(
		c.initLogger(),
		c.initEngineCommitmentSynchronization(),
	)

	protocol.Shutdown.OnTrigger(shutdown)

	return c
}

// Get returns the Commitment for the given commitmentID. If the Commitment is not available yet, it will return an
// ErrorCommitmentNotFound. It is possible to trigger a request for the Commitment by passing true as the second
// argument.
func (c *Commitments) Get(commitmentID iotago.CommitmentID, requestIfMissing ...bool) (commitment *Commitment, err error) {
	cachedRequest, exists := c.cachedRequests.Get(commitmentID)
	if !exists && lo.First(requestIfMissing) {
		if cachedRequest = c.cachedRequest(commitmentID, true); cachedRequest.WasRejected() {
			return nil, ierrors.Wrapf(cachedRequest.Err(), "failed to request commitment %s", commitmentID)
		}
	}

	if cachedRequest == nil || !cachedRequest.WasCompleted() {
		return nil, ErrorCommitmentNotFound
	}

	return cachedRequest.Result(), cachedRequest.Err()
}

// initLogger initializes the logger for this component.
func (c *Commitments) initLogger() (shutdown func()) {
	c.Logger, shutdown = c.protocol.NewChildLogger("Commitments")

	return lo.Batch(
		c.Root.LogUpdates(c, log.LevelTrace, "Root", (*Commitment).LogName),

		shutdown,
	)
}

// initEngineCommitmentSynchronization initializes the synchronization of commitments that are published by the engines.
func (c *Commitments) initEngineCommitmentSynchronization() func() {
	return c.protocol.Constructed.WithNonEmptyValue(func(_ bool) (shutdown func()) {
		return lo.Batch(
			// advance the root commitment of the main chain
			c.protocol.Chains.Main.WithNonEmptyValue(func(mainChain *Chain) (shutdown func()) {
				return mainChain.WithInitializedEngine(func(mainEngine *engine.Engine) (shutdown func()) {
					return c.publishRootCommitment(mainChain, mainEngine)
				})
			}),

			// publish the commitments that are produced by the engines
			c.protocol.Chains.WithInitializedEngines(func(chain *Chain, engine *engine.Engine) (shutdown func()) {
				return c.publishEngineCommitments(chain, engine)
			}),
		)
	})
}

// publishRootCommitment publishes the root commitment of the main engine.
func (c *Commitments) publishRootCommitment(mainChain *Chain, mainEngine *engine.Engine) func() {
	return mainEngine.RootCommitment.OnUpdate(func(_ *model.Commitment, newRootCommitmentModel *model.Commitment) {
		newRootCommitment, published, err := c.publishCommitmentModel(newRootCommitmentModel)
		if err != nil {
			c.LogError("failed to publish new root commitment", "id", newRootCommitmentModel.ID(), "error", err)

			return
		}

		newRootCommitment.IsRoot.Set(true)
		if published {
			newRootCommitment.Chain.Set(mainChain)
		}

		// TODO: USE SET HERE (debug eviction issues)
		mainChain.ForkingPoint.DefaultTo(newRootCommitment)

		c.Root.Set(newRootCommitment)
	})
}

// publishEngineCommitments publishes the commitments of the given engine to its chain.
func (c *Commitments) publishEngineCommitments(chain *Chain, engine *engine.Engine) (shutdown func()) {
	latestPublishedSlot := chain.LastCommonSlot()

	return engine.LatestCommitment.OnUpdate(func(_ *model.Commitment, latestCommitment *model.Commitment) {
		loadModel := func(slot iotago.SlotIndex) (*model.Commitment, error) {
			// prevent disk access if possible
			if slot == latestCommitment.Slot() {
				return latestCommitment, nil
			}

			return engine.Storage.Commitments().Load(slot)
		}

		for ; latestPublishedSlot < latestCommitment.Slot(); latestPublishedSlot++ {
			// retrieve the model to publish
			modelToPublish, err := loadModel(latestPublishedSlot + 1)
			if err != nil {
				c.LogError("failed to load commitment to publish from engine", "slot", latestPublishedSlot+1, "err", err)

				return
			}

			// publish the model
			publishedCommitment, _, err := c.publishCommitmentModel(modelToPublish)
			if err != nil {
				c.LogError("failed to publish commitment from engine", "engine", engine.LogName(), "commitment", modelToPublish, "err", err)

				return
			}

			// mark it as produced by ourselves and force it to be on the right chain (in case our chain produced a
			// different commitment than the one we erroneously expected it to be - we always trust our engine most).
			publishedCommitment.AttestedWeight.Set(publishedCommitment.Weight.Get())
			publishedCommitment.IsVerified.Set(true)
			publishedCommitment.forceChain(chain)
		}
	})
}

// publishCommitmentModel publishes the given commitment model as a Commitment instance. If the Commitment was already
// published, it will return the existing Commitment instance. Otherwise, it will create a new Commitment instance and
// resolve the Promise that was created for it.
func (c *Commitments) publishCommitmentModel(model *model.Commitment) (commitment *Commitment, published bool, err error) {
	// retrieve promise and abort if it was already rejected
	cachedRequest := c.cachedRequest(model.ID())
	if cachedRequest.WasRejected() {
		return nil, false, ierrors.Wrapf(cachedRequest.Err(), "failed to request commitment %s", model.ID())
	}

	// otherwise try to provideCommitment it and determine if we were the goroutine that resolved it
	commitment = newCommitment(c, model)
	cachedRequest.Resolve(commitment).OnSuccess(func(resolvedCommitment *Commitment) {
		if published = resolvedCommitment == commitment; !published {
			commitment = resolvedCommitment
		}
	})

	return commitment, published, nil
}

// cachedRequest returns a singleton Promise for the given commitmentID. If the Promise does not exist yet, it will be
// created and optionally requested from the network if missing. Once the promise is resolved, the Commitment is
// initialized and provided to the consumers.
func (c *Commitments) cachedRequest(commitmentID iotago.CommitmentID, requestIfMissing ...bool) *promise.Promise[*Commitment] {
	// handle evicted slots
	slotEvicted := c.protocol.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.protocol.LastEvictedSlot().Get() != 0 {
		return promise.New[*Commitment]().Reject(ErrorSlotEvicted)
	}

	// create a new promise or return the existing one
	cachedRequest, promiseCreated := c.cachedRequests.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if !promiseCreated {
		return cachedRequest
	}

	// start ticker if requested
	if lo.First(requestIfMissing) {
		c.protocol.CommitmentsProtocol.StartTicker(cachedRequest, commitmentID)
	}

	// handle successful resolutions
	cachedRequest.OnSuccess(func(commitment *Commitment) {
		c.initCommitment(commitment, slotEvicted)
	})

	// handle failed resolutions
	cachedRequest.OnError(func(err error) {
		c.LogDebug("request failed", "commitmentID", commitmentID, "error", err)
	})

	// tear down the promise once the slot is evicted
	slotEvicted.OnTrigger(func() {
		c.cachedRequests.Delete(commitmentID)

		cachedRequest.Reject(ErrorSlotEvicted)
	})

	return cachedRequest
}

// initCommitment initializes the given commitment.
func (c *Commitments) initCommitment(commitment *Commitment, slotEvicted reactive.Event) {
	commitment.LogDebug("created", "id", commitment.ID())

	// solidify the parent of the commitment
	c.cachedRequest(commitment.PreviousCommitmentID(), true).OnSuccess(func(parent *Commitment) {
		commitment.Parent.Set(parent)
	})

	// add commitment to the set
	c.Add(commitment)

	// tear down the commitment once the slot is evicted
	slotEvicted.OnTrigger(func() {
		c.Delete(commitment)

		commitment.IsEvicted.Trigger()
	})
}