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

	candidateChain reactive.Variable[*Chain]

	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	commitmentCreated *event.Event1[*CommitmentMetadata]

	chainCreated *event.Event1[*Chain]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainManager(rootCommitment *model.Commitment) *ChainManager {
	c := &ChainManager{
		mainChain:           reactive.NewVariable[*Chain](),
		candidateChain:      reactive.NewVariable[*Chain](),
		commitments:         shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]](),
		commitmentRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentCreated:   event.New1[*CommitmentMetadata](),
		chainCreated:        event.New1[*Chain](),
		EvictionState:       reactive.NewEvictionState[iotago.SlotIndex](),
	}

	c.mainChain.Set(NewChain(NewRootCommitmentMetadata(rootCommitment, c)))

	c.initChainSwitching()

	return c
}

func (c *ChainManager) initChainSwitching() {
	c.OnChainCreated(func(chain *Chain) {
		unsubscribe := chain.cumulativeWeight.OnUpdate(func(_, chainWeight uint64) {
			c.candidateChain.Compute(func(candidateChain *Chain) *Chain {
				if candidateChain == nil || candidateChain.evicted.WasTriggered() || chainWeight > candidateChain.CumulativeWeight().Get() {
					return chain
				}

				return candidateChain
			})
		})

		chain.evicted.OnTrigger(unsubscribe)
	})
}

func (c *ChainManager) ProcessCommitment(commitment *model.Commitment) (commitmentMetadata *CommitmentMetadata) {
	if commitmentRequest := c.requestCommitment(commitment.ID(), commitment.Index(), false, func(resolvedMetadata *CommitmentMetadata) {
		commitmentMetadata = resolvedMetadata
	}); commitmentRequest != nil {
		commitmentRequest.Resolve(NewCommitmentMetadata(commitment, c))
	}

	return commitmentMetadata
}

func (c *ChainManager) OnCommitmentCreated(callback func(commitment *CommitmentMetadata)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *ChainManager) OnChainCreated(callback func(chain *Chain)) (unsubscribe func()) {
	return c.chainCreated.Hook(callback).Unhook
}

func (c *ChainManager) MainChain() reactive.Variable[*Chain] {
	return c.mainChain
}

func (c *ChainManager) CandidateChain() reactive.Variable[*Chain] {
	return c.candidateChain
}

func (c *ChainManager) RootCommitment() reactive.Variable[*CommitmentMetadata] {
	chain := c.mainChain.Get()
	if chain == nil {
		panic("root chain not initialized")
	}

	return chain.Root()
}

func (c *ChainManager) setupCommitment(commitment *CommitmentMetadata, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, func(metadata *CommitmentMetadata) {
		commitment.Parent().Set(metadata)
	})

	slotEvictedEvent.OnTrigger(func() {
		commitment.Evicted().Trigger()
	})

	c.commitmentCreated.Trigger(commitment)
}

func (c *ChainManager) requestCommitment(commitmentID iotago.CommitmentID, index iotago.SlotIndex, requestFromPeers bool, optSuccessCallbacks ...func(metadata *CommitmentMetadata)) (commitmentRequest *promise.Promise[*CommitmentMetadata]) {
	slotEvicted := c.EvictionEvent(index)
	if slotEvicted.WasTriggered() {
		rootCommitment := c.mainChain.Get().root.Get()

		if rootCommitment == nil || commitmentID != rootCommitment.ID() {
			return nil
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(rootCommitment)
		}

		return promise.New[*CommitmentMetadata]().Resolve(rootCommitment)
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*CommitmentMetadata]))
	if requestCreated {
		if requestFromPeers {
			c.commitmentRequester.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.commitmentRequester.StopTicker(commitmentID)
			})
		}

		commitmentRequest.OnSuccess(func(commitment *CommitmentMetadata) {
			c.setupCommitment(commitment, slotEvicted)
		})

		slotEvicted.OnTrigger(func() { c.commitments.Delete(commitmentID) })
	}

	for _, successCallback := range optSuccessCallbacks {
		commitmentRequest.OnSuccess(successCallback)
	}

	return commitmentRequest
}
