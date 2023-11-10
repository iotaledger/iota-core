package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitments struct {
	reactive.Set[*Commitment]

	protocol    *Protocol
	commitments *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *promise.Promise[*Commitment]]
}

func newCommitments(protocol *Protocol) *Commitments {
	c := &Commitments{
		Set:         reactive.NewSet[*Commitment](),
		protocol:    protocol,
		commitments: shrinkingmap.New[iotago.CommitmentID, *promise.Promise[*Commitment]](),
	}

	return c
}

func (c *Commitments) Publish(commitment *model.Commitment) (commitmentMetadata *Commitment, published bool, err error) {
	request := c.requestCommitment(commitment.ID(), false)
	if request.WasRejected() {
		return nil, false, ierrors.Wrapf(request.Err(), "failed to request commitment %s", commitment.ID())
	}

	publishedCommitmentMetadata := NewCommitment(commitment, c.protocol.Chains)
	request.Resolve(publishedCommitmentMetadata).OnSuccess(func(resolvedMetadata *Commitment) {
		commitmentMetadata = resolvedMetadata
	})

	if published = commitmentMetadata == publishedCommitmentMetadata; published {
		commitmentMetadata.LogDebug("created", "id", commitment.ID())

		if c.Add(commitmentMetadata) {
			commitmentMetadata.IsEvicted.OnTrigger(func() { c.Delete(commitmentMetadata) })
		}
	}

	return commitmentMetadata, commitmentMetadata == publishedCommitmentMetadata, nil
}

func (c *Commitments) Get(commitmentID iotago.CommitmentID, requestMissing ...bool) (commitment *Commitment, err error) {
	commitmentRequest, exists := c.commitments.Get(commitmentID)
	if !exists && lo.First(requestMissing) {
		if commitmentRequest = c.requestCommitment(commitmentID, true); commitmentRequest.WasRejected() {
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

func (c *Commitments) requestCommitment(commitmentID iotago.CommitmentID, requestFromPeers bool, optSuccessCallbacks ...func(metadata *Commitment)) (commitmentRequest *promise.Promise[*Commitment]) {
	slotEvicted := c.protocol.EvictionEvent(commitmentID.Index())
	if slotEvicted.WasTriggered() && c.protocol.LastEvictedSlot().Get() != 0 {
		forkingPoint := c.protocol.Chains.Main.Get().ForkingPoint.Get()

		if forkingPoint == nil || commitmentID != forkingPoint.ID() {
			return promise.New[*Commitment]().Reject(ErrorSlotEvicted)
		}

		for _, successCallback := range optSuccessCallbacks {
			successCallback(forkingPoint)
		}

		return promise.New[*Commitment]().Resolve(forkingPoint)
	}

	commitmentRequest, requestCreated := c.commitments.GetOrCreate(commitmentID, lo.NoVariadic(promise.New[*Commitment]))
	if requestCreated {
		if requestFromPeers {
			c.protocol.CommitmentsProtocol.ticker.StartTicker(commitmentID)

			commitmentRequest.OnComplete(func() {
				c.protocol.CommitmentsProtocol.ticker.StopTicker(commitmentID)
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

func (c *Commitments) setupCommitment(commitment *Commitment, slotEvictedEvent reactive.Event) {
	c.requestCommitment(commitment.PreviousCommitmentID(), true, lo.Void(commitment.Parent.Set)).OnError(func(err error) {
		c.protocol.LogDebug("failed to request previous commitment", "prevId", commitment.PreviousCommitmentID(), "error", err)
	})

	slotEvictedEvent.OnTrigger(func() {
		commitment.IsEvicted.Trigger()
	})

	c.Add(commitment)
}
