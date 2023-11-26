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

	// cachedRequests contains all cached requests for commitments that are currently relevant.
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
		c.initLogger(protocol.NewChildLogger("Commitments")),
		c.initEngineCommitmentSynchronization(protocol),
	)

	protocol.Shutdown.OnTrigger(shutdown)

	return c
}

// Publish publishes the given commitment model to the collection and returns the corresponding Commitment singleton
// that holds the metadata.
func (c *Commitments) Publish(commitment *model.Commitment) (commitmentMetadata *Commitment, published bool, err error) {
	// create cached request
	cachedRequest := c.createCachedRequest(commitment.ID(), false)
	if cachedRequest.WasRejected() {
		return nil, false, ierrors.Wrapf(cachedRequest.Err(), "failed to request commitment %s", commitment.ID())
	}

	publishedCommitmentMetadata := newCommitment(commitment, c.protocol)
	cachedRequest.Resolve(publishedCommitmentMetadata).OnSuccess(func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	})

	return commitmentMetadata, commitmentMetadata == publishedCommitmentMetadata, nil
}

func (c *Commitments) Get(commitmentID iotago.CommitmentID, requestMissing ...bool) (commitment *Commitment, err error) {
	commitmentRequest, exists := c.cachedRequests.Get(commitmentID)
	if !exists && lo.First(requestMissing) {
		if commitmentRequest = c.createCachedRequest(commitmentID, true); commitmentRequest.WasRejected() {
			return nil, ierrors.Wrapf(commitmentRequest.Err(), "failed to request commitment %s", commitmentID)
		}
	}

	if commitmentRequest == nil || !commitmentRequest.WasCompleted() {
		return nil, ErrorCommitmentNotFound
	}

	if commitmentRequest.WasRejected() {
		return nil, commitmentRequest.Err()
	}

	return commitmentRequest.Result(), nil
}

func (c *Commitments) initLogger(logger log.Logger, shutdownLogger func()) (teardown func()) {
	c.Logger = logger

	return lo.Batch(
		c.Root.LogUpdates(c, log.LevelTrace, "Root", (*Commitment).LogName),

		shutdownLogger,
	)
}

func (c *Commitments) initEngineCommitmentSynchronization(protocol *Protocol) func() {
	return protocol.Constructed.WithNonEmptyValue(func(_ bool) (teardown func()) {
		return lo.Batch(
			protocol.Chains.Main.WithNonEmptyValue(func(mainChain *Chain) (teardown func()) {
				return mainChain.WithInitializedEngine(func(mainEngine *engine.Engine) (teardown func()) {
					return c.publishRootCommitment(mainChain, mainEngine)
				})
			}),

			protocol.Chains.WithInitializedEngines(func(chain *Chain, engine *engine.Engine) (teardown func()) {
				return c.publishEngineCommitments(chain, engine)
			}),
		)
	})
}

func (c *Commitments) publishRootCommitment(mainChain *Chain, mainEngine *engine.Engine) func() {
	return mainEngine.RootCommitment.OnUpdate(func(_ *model.Commitment, newRootCommitmentModel *model.Commitment) {
		newRootCommitment, published, err := c.Publish(newRootCommitmentModel)
		if err != nil {
			c.LogError("failed to publish new root commitment", "id", newRootCommitmentModel.ID(), "error", err)

			return
		}

		newRootCommitment.IsRoot.Set(true)
		if published {
			newRootCommitment.Chain.Set(mainChain)
		}

		// TODO: SET HERE AND FIX BUG
		mainChain.ForkingPoint.DefaultTo(newRootCommitment)

		c.Root.Set(newRootCommitment)
	})
}

func (c *Commitments) publishEngineCommitments(chain *Chain, engine *engine.Engine) (teardown func()) {
	return engine.LatestCommitment.OnUpdate(func(_ *model.Commitment, latestCommitment *model.Commitment) {
		for latestPublishedSlot := chain.LastCommonSlot(); latestPublishedSlot < latestCommitment.Slot(); latestPublishedSlot++ {
			modelToPublish, err := engine.Storage.Commitments().Load(latestPublishedSlot + 1)
			if err != nil {
				c.LogError("failed to load commitment to publish from engine", "slot", latestPublishedSlot+1, "err", err)

				return
			}

			publishedCommitment, _, err := c.Publish(modelToPublish)
			if err != nil {
				c.LogError("failed to publish commitment from engine", "engine", engine.LogName(), "commitment", modelToPublish, "err", err)

				return
			}

			publishedCommitment.AttestedWeight.Set(publishedCommitment.Weight.Get())
			publishedCommitment.IsVerified.Set(true)
			publishedCommitment.forceChain(chain)
		}
	})
}

func (c *Commitments) createCachedRequest(commitmentID iotago.CommitmentID, requestFromPeers bool) (commitmentRequest *promise.Promise[*Commitment]) {
	// handle evicted slots
	slotEvicted := c.protocol.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.protocol.LastEvictedSlot().Get() != 0 {
		return promise.New[*Commitment]().Reject(ErrorSlotEvicted)
	}

	// attempt to create a request and abort if it already exists
	commitmentRequest, requestCreated := c.cachedRequests.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if !requestCreated {
		return commitmentRequest
	}

	// start ticker if necessary
	if requestFromPeers {
		c.protocol.CommitmentsProtocol.StartTicker(commitmentRequest, commitmentID)
	}

	// handle successful requests
	commitmentRequest.OnSuccess(func(commitment *Commitment) {
		commitment.LogDebug("created", "id", commitment.ID())

		// solidify the parent of the commitment
		c.createCachedRequest(commitment.PreviousCommitmentID(), true).OnSuccess(func(parent *Commitment) {
			commitment.Parent.Set(parent)
		})

		// add commitment to the set
		c.Add(commitment)

		// tear down the commitment once the slot is evicted
		slotEvicted.OnTrigger(func() {
			c.Delete(commitment)

			commitment.IsEvicted.Trigger()
		})
	})

	// handle failed requests
	commitmentRequest.OnError(func(err error) {
		c.LogDebug("request failed", "commitmentID", commitmentID, "error", err)
	})

	// tear down the request once the slot is evicted
	slotEvicted.OnTrigger(func() {
		c.cachedRequests.Delete(commitmentID)

		commitmentRequest.Reject(ErrorSlotEvicted)
	})

	return commitmentRequest
}
