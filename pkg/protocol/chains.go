package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chains struct {
	protocol *Protocol

	mainChain reactive.Variable[*Chain]

	heaviestClaimedCandidate reactive.Variable[*Chain]

	heaviestAttestedCandidate reactive.Variable[*Chain]

	heaviestVerifiedCandidate reactive.Variable[*Chain]

	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	commitmentCreated *event.Event1[*Commitment]

	chainCreated *event.Event1[*Chain]

	reactive.EvictionState[iotago.SlotIndex]
}

func newChains(protocol *Protocol) *Chains {
	c := &Chains{
		protocol:                  protocol,
		EvictionState:             reactive.NewEvictionState[iotago.SlotIndex](),
		mainChain:                 reactive.NewVariable[*Chain]().Init(NewChain(NewCommitment(protocol.MainEngineInstance().SyncManager.LatestCommitment(), true), protocol.MainEngineInstance())),
		heaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		heaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		heaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		commitments:               shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		commitmentCreated:         event.New1[*Commitment](),
		chainCreated:              event.New1[*Chain](),
	}

	c.publishLatestEngineCommitment(protocol.MainEngineInstance())
	protocol.OnEngineCreated(c.publishLatestEngineCommitment)

	c.initChainSwitching()

	return c
}

func (c *Chains) PublishCommitment(commitment *model.Commitment) (commitmentMetadata *Commitment, err error) {
	request, requestErr := c.requestCommitment(commitment.ID(), false)
	if requestErr != nil {
		return nil, ierrors.Wrapf(requestErr, "failed to request commitment %s", commitment.ID())
	}

	request.Resolve(NewCommitment(commitment)).OnSuccess(func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	})

	return commitmentMetadata, nil
}

func (c *Chains) Commitment(commitmentID iotago.CommitmentID, requestMissing ...bool) (commitment *Commitment, err error) {
	commitmentRequest, exists := c.commitments.Get(commitmentID)
	if !exists && lo.First(requestMissing) {
		if commitmentRequest, err = c.requestCommitment(commitmentID, true); err != nil {
			return nil, ierrors.Wrapf(err, "failed to request commitment %s", commitmentID)
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

func (c *Chains) OnCommitmentCreated(callback func(commitment *Commitment)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *Chains) OnChainCreated(callback func(chain *Chain)) (unsubscribe func()) {
	return c.chainCreated.Hook(callback).Unhook
}

func (c *Chains) MainChain() *Chain {
	return c.mainChain.Get()
}

func (c *Chains) MainChainR() reactive.Variable[*Chain] {
	return c.mainChain
}

func (c *Chains) HeaviestClaimedCandidate() reactive.Variable[*Chain] {
	return c.heaviestClaimedCandidate
}

func (c *Chains) HeaviestAttestedCandidate() reactive.Variable[*Chain] {
	return c.heaviestAttestedCandidate
}

func (c *Chains) HeaviestVerifiedCandidate() reactive.Variable[*Chain] {
	return c.heaviestVerifiedCandidate
}

func (c *Chains) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	if _, err := c.requestCommitment(commitment.PrevID(), true, commitment.setParent); err != nil {
		c.protocol.LogDebug(ierrors.Wrapf(err, "failed to request parent commitment %s", commitment.PrevID()))
	}

	slotEvictedEvent.OnTrigger(func() {
		commitment.evicted.Trigger()
	})

	c.commitmentCreated.Trigger(commitment)

	commitment.spawnedChain.OnUpdate(func(_, newChain *Chain) {
		if newChain != nil {
			c.chainCreated.Trigger(newChain)
		}
	})
}

func (c *Chains) requestCommitment(commitmentID iotago.CommitmentID, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment], err error) {
	slotEvicted := c.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() {
		rootCommitment := c.mainChain.Get().Root()

		if rootCommitment == nil || commitmentID != rootCommitment.ID() {
			return nil, ErrorSlotEvicted
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(rootCommitment)
		}

		return promise.New[*Commitment]().Resolve(rootCommitment), nil
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if requestCreated {
		if requestFromPeers {
			c.protocol.commitmentRequester.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.protocol.commitmentRequester.StopTicker(commitmentID)
			})
		}

		commitmentRequest.OnSuccess(func(commitment *Commitment) {
			c.setupCommitment(commitment, slotEvicted)
		})

		slotEvicted.OnTrigger(func() { c.commitments.Delete(commitmentID) })
	}

	for _, successCallback := range optSuccessCallbacks {
		commitmentRequest.OnSuccess(successCallback)
	}

	return commitmentRequest, nil
}

func (c *Chains) publishLatestEngineCommitment(engine *engine.Engine) {
	unsubscribe := engine.Events.Notarization.LatestCommitmentUpdated.Hook(func(modelCommitment *model.Commitment) {
		commitment, err := c.PublishCommitment(modelCommitment)
		if err != nil {
			c.protocol.LogDebug(ierrors.Wrapf(err, "failed to add commitment %s", modelCommitment.ID()))
		}

		commitment.Parent().Get().successor.Set(commitment)

		commitment.Verified().Trigger()
	}).Unhook

	engine.HookShutdown(unsubscribe)
}

func (c *Chains) initChainSwitching() {
	c.heaviestClaimedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.requestAttestations.Set(false)
		}

		newCandidate.requestAttestations.Set(true)
	})

	c.heaviestAttestedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.engine.instantiate.Set(false)
		}

		newCandidate.engine.instantiate.Set(true)
	})

	c.OnChainCreated(func(chain *Chain) {
		c.trackHeaviestCandidate(c.heaviestClaimedCandidate, (*Chain).ClaimedWeight, chain)
		c.trackHeaviestCandidate(c.heaviestAttestedCandidate, (*Chain).AttestedWeight, chain)
		c.trackHeaviestCandidate(c.heaviestVerifiedCandidate, (*Chain).VerifiedWeight, chain)
	})
}

func (c *Chains) trackHeaviestCandidate(candidateVariable reactive.Variable[*Chain], chainWeightVariable func(*Chain) reactive.Variable[uint64], candidate *Chain) {
	chainWeightVariable(candidate).OnUpdate(func(_, newChainWeight uint64) {
		if newChainWeight <= c.mainChain.Get().verifiedWeight.Get() {
			return
		}

		candidateVariable.Compute(func(currentCandidate *Chain) *Chain {
			if currentCandidate == nil || currentCandidate.evicted.WasTriggered() || newChainWeight > chainWeightVariable(currentCandidate).Get() {
				return candidate
			}

			return currentCandidate
		})
	})
}
