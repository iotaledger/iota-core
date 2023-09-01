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

	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]

	commitmentCreated *event.Event1[*Commitment]

	chainCreated *event.Event1[*Chain]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	*EngineManager

	*ChainSwitching

	*AttestationsRequester

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainManager(rootCommitment *model.Commitment) *ChainManager {
	p := &ChainManager{
		EvictionState:       reactive.NewEvictionState[iotago.SlotIndex](),
		mainChain:           reactive.NewVariable[*Chain]().Init(NewChain(NewCommitment(rootCommitment, true))),
		commitments:         shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
		commitmentCreated:   event.New1[*Commitment](),
		chainCreated:        event.New1[*Chain](),
		commitmentRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
	}

	// embed reactive orchestrators
	p.EngineManager = NewEngineManager(p)
	p.ChainSwitching = NewChainSwitching(p)
	p.AttestationsRequester = NewAttestationsRequester(p)

	return p
}

func (c *ChainManager) MainChain() reactive.Variable[*Chain] {
	return c.mainChain
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

func (c *ChainManager) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.setParent)

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
