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
	rootChain reactive.Variable[*Chain]

	commitmentCreated *event.Event1[*CommitmentMetadata]

	cachedCommitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainManager(rootCommitment *model.Commitment) *ChainManager {
	return &ChainManager{
		EvictionState:       reactive.NewEvictionState[iotago.SlotIndex](),
		rootChain:           reactive.NewVariable[*Chain]().Init(NewChain(NewRootCommitmentMetadata(rootCommitment))),
		commitmentCreated:   event.New1[*CommitmentMetadata](),
		cachedCommitments:   shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]](),
		commitmentRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
	}
}

func (c *ChainManager) ProcessCommitment(commitment *model.Commitment) (commitmentMetadata *CommitmentMetadata) {
	if commitmentRequest := c.requestCommitment(commitment.ID(), commitment.Index(), false, func(resolvedMetadata *CommitmentMetadata) {
		commitmentMetadata = resolvedMetadata
	}); commitmentRequest != nil {
		commitmentRequest.Resolve(NewCommitmentMetadata(commitment))
	}

	return commitmentMetadata
}

func (c *ChainManager) OnCommitmentCreated(callback func(commitment *CommitmentMetadata)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *ChainManager) RootCommitment() *CommitmentMetadata {
	chain := c.rootChain.Get()
	if chain == nil {
		return nil
	}

	return chain.ForkingPoint().Get()
}

func (c *ChainManager) setupCommitment(commitment *CommitmentMetadata, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.registerParent)

	slotEvictedEvent.OnTrigger(func() { commitment.Evicted().Trigger() })

	c.commitmentCreated.Trigger(commitment)
}

func (c *ChainManager) requestCommitment(id iotago.CommitmentID, index iotago.SlotIndex, requestIfMissing bool, optSuccessCallbacks ...func(metadata *CommitmentMetadata)) (commitmentRequest *promise.Promise[*CommitmentMetadata]) {
	slotEvicted := c.EvictionEvent(index)
	if slotEvicted.WasTriggered() {
		if rootCommitment := c.RootCommitment(); rootCommitment != nil && id == rootCommitment.ID() {
			for _, successCallback := range optSuccessCallbacks {
				successCallback(rootCommitment)
			}

			return promise.New[*CommitmentMetadata]().Resolve(rootCommitment)
		}

		return nil
	}

	commitmentRequest, requestCreated := c.cachedCommitments.GetOrCreate(id, lo.NoVariadic(promise.New[*CommitmentMetadata]))
	if requestCreated {
		if requestIfMissing {
			c.commitmentRequester.StartTicker(id)
		}

		commitmentRequest.OnSuccess(func(commitment *CommitmentMetadata) {
			if requestIfMissing {
				c.commitmentRequester.StopTicker(commitment.ID())
			}

			c.setupCommitment(commitment, slotEvicted)
		})

		slotEvicted.OnTrigger(func() { c.cachedCommitments.Delete(id) })
	}

	for _, successCallback := range optSuccessCallbacks {
		commitmentRequest.OnSuccess(successCallback)
	}

	return commitmentRequest
}
