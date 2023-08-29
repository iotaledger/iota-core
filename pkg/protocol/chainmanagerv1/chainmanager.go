package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainManager struct {
	mainChain reactive.Variable[*Chain]

	heaviestClaimedCandidate reactive.Variable[*Chain]

	heaviestAttestedCandidate reactive.Variable[*Chain]

	heaviestVerifiedCandidate reactive.Variable[*Chain]

	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	commitmentCreated *event.Event1[*Commitment]

	chainCreated *event.Event1[*Chain]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainManager(rootCommitment *model.Commitment) *ChainManager {
	c := &ChainManager{
		mainChain:                 reactive.NewVariable[*Chain]().Init(NewChain(NewRootCommitment(rootCommitment))),
		heaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		heaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		heaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
		commitments:               shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		commitmentRequester:       eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentCreated:         event.New1[*Commitment](),
		chainCreated:              event.New1[*Chain](),
		EvictionState:             reactive.NewEvictionState[iotago.SlotIndex](),
	}

	c.OnChainCreated(func(chain *Chain) {
		c.selectHeaviestCandidate(c.heaviestClaimedCandidate, chain, (*Chain).ClaimedWeightVariable)
		c.selectHeaviestCandidate(c.heaviestAttestedCandidate, chain, (*Chain).AttestedWeightVariable)
		c.selectHeaviestCandidate(c.heaviestVerifiedCandidate, chain, (*Chain).VerifiedWeightVariable)
	})

	return c
}

func (c *ChainManager) ProcessCommitment(commitment *model.Commitment) (commitmentMetadata *Commitment) {
	if commitmentRequest := c.requestCommitment(commitment.ID(), commitment.Index(), false, func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	}); commitmentRequest != nil {
		commitmentRequest.Resolve(NewCommitment(commitment))
	}

	return commitmentMetadata
}

func (c *ChainManager) OnCommitmentCreated(callback func(commitment *Commitment)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *ChainManager) OnChainCreated(callback func(chain *Chain)) (unsubscribe func()) {
	return c.chainCreated.Hook(callback).Unhook
}

func (c *ChainManager) MainChain() *Chain {
	return c.mainChain.Get()
}

func (c *ChainManager) MainChainVar() reactive.Variable[*Chain] {
	return c.mainChain
}

func (c *ChainManager) HeaviestCandidateChain() reactive.Variable[*Chain] {
	return c.heaviestClaimedCandidate
}

func (c *ChainManager) HeaviestAttestedCandidateChain() reactive.Variable[*Chain] {
	return c.heaviestAttestedCandidate
}

func (c *ChainManager) HeaviestVerifiedCandidateChain() reactive.Variable[*Chain] {
	return c.heaviestVerifiedCandidate
}

func (c *ChainManager) selectHeaviestCandidate(variable reactive.Variable[*Chain], newCandidate *Chain, chainWeight func(*Chain) reactive.Variable[uint64]) {
	chainWeight(newCandidate).OnUpdate(func(_, newChainWeight uint64) {
		if newChainWeight <= c.MainChain().VerifiedWeight() {
			return
		}

		variable.Compute(func(currentCandidate *Chain) *Chain {
			if currentCandidate == nil || currentCandidate.evicted.WasTriggered() || newChainWeight > chainWeight(currentCandidate).Get() {
				return newCandidate
			}

			return currentCandidate
		})
	})
}

func (c *ChainManager) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.setParent)

	slotEvictedEvent.OnTrigger(func() {
		commitment.evicted.Trigger()
	})

	c.commitmentCreated.Trigger(commitment)

	commitment.SpawnedChainVariable().OnUpdate(func(_, newChain *Chain) {
		if newChain != nil {
			c.chainCreated.Trigger(newChain)
		}
	})
}

func (c *ChainManager) requestCommitment(commitmentID iotago.CommitmentID, index iotago.SlotIndex, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment]) {
	slotEvicted := c.EvictionEvent(index)
	if slotEvicted.WasTriggered() {
		rootCommitment := c.mainChain.Get().Root()

		if rootCommitment == nil || commitmentID != rootCommitment.ID() {
			return nil
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(rootCommitment)
		}

		return promise.New[*Commitment]().Resolve(rootCommitment)
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if requestCreated {
		if requestFromPeers {
			c.commitmentRequester.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.commitmentRequester.StopTicker(commitmentID)
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

	return commitmentRequest
}
