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

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainManager() *ChainManager {
	return &ChainManager{
		EvictionState:       reactive.NewEvictionState[iotago.SlotIndex](),
		rootCommitment:      reactive.NewVariable[*CommitmentMetadata](),
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

		return commitmentMetadata
	})

	if err != nil {
		return nil, err
	}

	return commitmentMetadata, nil
}

func (c *ChainManager) setupCommitment(commitment *CommitmentMetadata, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PrevID(), commitment.Index()-1, true, commitment.registerParent)

	slotEvictedEvent.OnTrigger(func() { commitment.Evicted().Trigger() })

	c.commitmentCreated.Trigger(commitment)
}

func (c *ChainManager) requestCommitment(id iotago.CommitmentID, index iotago.SlotIndex, requestIfMissing bool, optSuccessCallbacks ...func(metadata *CommitmentMetadata)) (commitmentRequest *promise.Promise[*CommitmentMetadata]) {
	slotEvicted := c.EvictionEvent(index)
	if slotEvicted.WasTriggered() {
		if rootCommitment := c.rootCommitment.Get(); rootCommitment != nil && id == rootCommitment.ID() {
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
