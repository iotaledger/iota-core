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
		rootChain:            NewChain(),
		rootCommitment:       reactive.NewVariable[*CommitmentMetadata](),
		commitmentCreated:    event.New1[*CommitmentMetadata](),
		cachedCommitments:    shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*CommitmentMetadata]](),
		commitmentRequester:  eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		slotEvictionEvents:   shrinkingmap.New[iotago.SlotIndex, reactive.Event](),
		lastEvictedSlotIndex: reactive.NewVariable[iotago.SlotIndex](),
	}
}

func (c *ChainManager) SetRootCommitment(commitment *model.Commitment) (commitmentMetadata *CommitmentMetadata, err error) {
	var prevRootToEvict *CommitmentMetadata

	c.rootCommitment.Compute(func(currentRoot *CommitmentMetadata) *CommitmentMetadata {
		if prevRootToEvict = currentRoot; currentRoot != nil {
			if currentRoot.Index() > commitment.Index() {
				err = ierrors.Errorf("cannot set root commitment with lower index %s than current root commitment index %s", commitment.Index(), currentRoot.Index())
				return currentRoot
			}

			if currentRoot.Index() == commitment.Index() && currentRoot.ID() != commitment.ID() {
				err = ierrors.Errorf("cannot set root commitment with same index %s but different id %s than current root commitment id %s", commitment.Index(), commitment.ID(), currentRoot.ID())
				return currentRoot
			}
		}

		commitmentRequest, requestCreated := c.cachedCommitments.GetOrCreate(commitment.ID(), lo.NoVariadic(promise.New[*CommitmentMetadata]))
		commitmentRequest.OnSuccess(func(resolvedMetadata *CommitmentMetadata) {
			commitmentMetadata = resolvedMetadata
			commitmentMetadata.Chain().Set(c.rootChain)
			commitmentMetadata.Solid().Trigger()
			commitmentMetadata.Verified().Trigger()
			commitmentMetadata.BelowSyncThreshold().Trigger()
			commitmentMetadata.BelowWarpSyncThreshold().Trigger()
			commitmentMetadata.BelowLatestVerifiedCommitment().Trigger()

			if requestCreated {
				c.commitmentCreated.Trigger(commitmentMetadata)

				commitmentMetadata.Evicted().OnTrigger(func() {
					c.cachedCommitments.Delete(commitment.ID())
				})
			}
		})
		commitmentRequest.Resolve(NewCommitmentMetadata(commitment))

		return commitmentMetadata
	})

	if err != nil {
		return nil, err
	}

	if prevRootToEvict != nil {
		prevRootToEvict.Evicted().Trigger()
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

func (c *ChainManager) SlotEvictedEvent(index iotago.SlotIndex) (slotEvictedEvent reactive.Event) {
	c.lastEvictedSlotIndex.Compute(func(lastEvictedSlotIndex iotago.SlotIndex) iotago.SlotIndex {
		if index > lastEvictedSlotIndex {
			slotEvictedEvent, _ = c.slotEvictionEvents.GetOrCreate(index, reactive.NewEvent)
		} else {
			slotEvictedEvent = triggeredEvent
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
	if rootCommitment := c.rootCommitment.Get(); rootCommitment != nil && commitment.PrevID() == rootCommitment.ID() {
		commitment.RegisterParent(rootCommitment)
	} else {
		c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.RegisterParent)
	}

	slotEvictedEvent.OnTrigger(func() {
		if c.rootCommitment.Get().ID() != commitment.ID() {
			commitment.Evicted().Trigger()
		}
	})

	c.commitmentCreated.Trigger(commitment)
}

func (c *ChainManager) requestCommitment(id iotago.CommitmentID, index iotago.SlotIndex, requestIfMissing bool, optSuccessCallbacks ...func(metadata *CommitmentMetadata)) (commitmentRequest *promise.Promise[*CommitmentMetadata], requestCreated bool) {
	rootCommitment := c.rootCommitment.Get()

	if slotEvictedEvent := c.SlotEvictedEvent(index); !slotEvictedEvent.WasTriggered() || rootCommitment != nil && rootCommitment.ID() == id {
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

			slotEvictedEvent.OnTrigger(func() {
				if rootCommitment == nil || id != c.rootCommitment.Get().ID() {
					c.cachedCommitments.Delete(id)
				} else {
					rootCommitment.Evicted().OnTrigger(func() {
						c.cachedCommitments.Delete(id)
					})
				}
			})
		}

		for _, successCallback := range optSuccessCallbacks {
			commitmentRequest.OnSuccess(successCallback)
		}
	}

	return commitmentRequest, requestCreated
}

var triggeredEvent = reactive.NewEvent()

func init() {
	triggeredEvent.Trigger()
}
