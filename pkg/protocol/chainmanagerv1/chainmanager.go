package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainManager struct {
	rootChain *Chain

	rootCommitment reactive.Variable[*CommitmentMetadata]

	commitmentCreated *event.Event1[*CommitmentMetadata]

	cachedCommitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	slotEvictionEvents *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.Event]

	lastEvictedSlotIndex reactive.Variable[iotago.SlotIndex]
}

func NewChainManager() *ChainManager {
	return &ChainManager{
		rootCommitment:       reactive.NewVariable[*CommitmentMetadata](),
		commitmentCreated:    event.New1[*CommitmentMetadata](),
		cachedCommitments:    shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]](),
		commitmentRequester:  eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		slotEvictionEvents:   shrinkingmap.New[iotago.SlotIndex, reactive.Event](),
		lastEvictedSlotIndex: reactive.NewVariable[iotago.SlotIndex](),
	}
}

func (c *ChainManager) SetRootCommitment(commitment *model.Commitment) (commitmentMetadata *CommitmentMetadata, err error) {
	c.rootCommitment.Compute(func(currentRoot *CommitmentMetadata) *CommitmentMetadata {
		if currentRoot != nil {
			if currentRoot.Index() > commitment.Index() {
				err = ierrors.Errorf("cannot set root commitment with lower index %s than current root commitment index %s", commitment.Index(), currentRoot.Index())
				return currentRoot
			}

			if currentRoot.Index() == commitment.Index() && currentRoot.ID() != commitment.ID() {
				err = ierrors.Errorf("cannot set root commitment with same index %s but different id %s than current root commitment id %s", commitment.Index(), commitment.ID(), currentRoot.ID())
				return currentRoot
			}
		}

		commitmentMetadata = NewCommitmentMetadata(commitment)
		commitmentMetadata.Solid().Trigger()
		commitmentMetadata.Verified().Trigger()
		commitmentMetadata.BelowSyncThreshold().Trigger()
		commitmentMetadata.BelowWarpSyncThreshold().Trigger()
		commitmentMetadata.BelowLatestVerifiedCommitment().Trigger()
		commitmentMetadata.Evicted().Trigger()

		c.rootChain = NewChain(commitmentMetadata)
		commitmentMetadata.Chain().Set(c.rootChain)

		return commitmentMetadata
	})

	if err != nil {
		return nil, err
	}

	return commitmentMetadata, nil
}

func (c *ChainManager) ProcessCommitment(commitment *model.Commitment) (commitmentMetadata *CommitmentMetadata) {
	if commitmentRequest, _ := c.requestCommitment(commitment.ID(), commitment.Index(), false, func(resolvedMetadata *CommitmentMetadata) {
		commitmentMetadata = resolvedMetadata
	}); commitmentRequest != nil {
		commitmentRequest.Resolve(NewCommitmentMetadata(commitment))
	}

	return commitmentMetadata
}

func (c *ChainManager) OnCommitmentCreated(callback func(commitment *CommitmentMetadata)) (unsubscribe func()) {
	return c.commitmentCreated.Hook(callback).Unhook
}

func (c *ChainManager) SlotEvictedEvent(index iotago.SlotIndex) reactive.Event {
	var slotEvictedEvent reactive.Event

	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex iotago.SlotIndex) iotago.SlotIndex {
		if index > lastEvictedSlotIndex {
			slotEvictedEvent, _ = c.slotEvictionEvents.GetOrCreate(index, reactive.NewEvent)
		} else {
			slotEvictedEvent = defaultTriggeredEvent
		}

		return lastEvictedSlotIndex
	})

	return slotEvictedEvent
}

func (c *ChainManager) Evict(slotIndex iotago.SlotIndex) {
	slotEvictedEventsToTrigger := make([]reactive.Event, 0)

	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex iotago.SlotIndex) iotago.SlotIndex {
		if slotIndex <= lastEvictedSlotIndex {
			return lastEvictedSlotIndex
		}

		for i := lastEvictedSlotIndex + 1; i <= slotIndex; i++ {
			if slotEvictedEvent, exists := c.slotEvictionEvents.Get(i); exists {
				slotEvictedEventsToTrigger = append(slotEvictedEventsToTrigger, slotEvictedEvent)
			}
		}

		return slotIndex
	})

	for _, slotEvictedEvent := range slotEvictedEventsToTrigger {
		slotEvictedEvent.Trigger()
	}
}

func (c *ChainManager) setupCommitment(commitment *CommitmentMetadata, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.RegisterParent)

	slotEvictedEvent.OnTrigger(func() { commitment.Evicted().Trigger() })

	c.commitmentCreated.Trigger(commitment)
}

func (c *ChainManager) requestCommitment(id iotago.CommitmentID, index iotago.SlotIndex, requestIfMissing bool, optSuccessCallbacks ...func(metadata *CommitmentMetadata)) (commitmentRequest *promise.Promise[*CommitmentMetadata], requestCreated bool) {
	slotEvictedEvent := c.SlotEvictedEvent(index)
	if slotEvictedEvent.WasTriggered() {
		if rootCommitment := c.rootCommitment.Get(); rootCommitment != nil && id == rootCommitment.ID() {
			for _, successCallback := range optSuccessCallbacks {
				successCallback(rootCommitment)
			}

			return promise.New[*CommitmentMetadata]().Resolve(rootCommitment), false
		}

		return nil, false
	}

	if commitmentRequest, requestCreated = c.cachedCommitments.GetOrCreate(id, lo.NoVariadic(promise.New[*CommitmentMetadata])); requestCreated {
		if requestIfMissing {
			c.commitmentRequester.StartTicker(id)
		}

		commitmentRequest.OnSuccess(func(commitment *CommitmentMetadata) {
			if requestIfMissing {
				c.commitmentRequester.StopTicker(commitment.ID())
			}

			c.setupCommitment(commitment, slotEvictedEvent)
		})

		slotEvictedEvent.OnTrigger(func() { c.cachedCommitments.Delete(id) })
	}

	for _, successCallback := range optSuccessCallbacks {
		commitmentRequest.OnSuccess(successCallback)
	}

	return commitmentRequest, requestCreated
}

var defaultTriggeredEvent = reactive.NewEvent()

func init() {
	defaultTriggeredEvent.Trigger()
}
