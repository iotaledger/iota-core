package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type commitmentThresholds struct {
	isAboveLatestVerifiedIndex reactive.Variable[bool]
	isBelowLatestVerifiedIndex reactive.Event
	isBelowSyncThreshold       reactive.Event
	isBelowWarpSyncThreshold   reactive.Event
}

func newCommitmentThresholds(commitment *Commitment) *commitmentThresholds {
	c := &commitmentThresholds{
		isAboveLatestVerifiedIndex: isAboveLatestVerifiedIndex(commitment),
		isBelowLatestVerifiedIndex: reactive.NewEvent(),
		isBelowSyncThreshold:       reactive.NewEvent(),
		isBelowWarpSyncThreshold:   reactive.NewEvent(),
	}

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold((*Commitment).IsBelowLatestVerifiedIndexEvent, commitment, (*Chain).LatestVerifiedCommitmentIndex)
		triggerEventIfCommitmentBelowThreshold((*Commitment).IsBelowSyncThresholdEvent, commitment, (*Chain).SyncThreshold)
		triggerEventIfCommitmentBelowThreshold((*Commitment).IsBelowWarpSyncThresholdEvent, commitment, (*Chain).WarpSyncThreshold)
	})

	return c
}

func (c *commitmentThresholds) IsAboveLatestVerifiedIndex() bool {
	return c.isAboveLatestVerifiedIndex.Get()
}

func (c *commitmentThresholds) IsAboveLatestVerifiedIndexVariable() reactive.Variable[bool] {
	return c.isAboveLatestVerifiedIndex
}

func (c *commitmentThresholds) IsBelowLatestVerifiedIndex() bool {
	return c.isBelowLatestVerifiedIndex.Get()
}

func (c *commitmentThresholds) IsBelowLatestVerifiedIndexEvent() reactive.Event {
	return c.isBelowLatestVerifiedIndex
}

func (c *commitmentThresholds) IsBelowSyncThreshold() bool {
	return c.isBelowSyncThreshold.Get()
}

func (c *commitmentThresholds) IsBelowSyncThresholdEvent() reactive.Event {
	return c.isBelowSyncThreshold
}

func (c *commitmentThresholds) IsBelowWarpSyncThreshold() bool {
	return c.isBelowWarpSyncThreshold.Get()
}

func (c *commitmentThresholds) IsBelowWarpSyncThresholdEvent() reactive.Event {
	return c.isBelowWarpSyncThreshold
}

func isAboveLatestVerifiedIndex(commitment *Commitment) reactive.Variable[bool] {
	var (
		parentVerified = reactive.NewEvent()

		parentAboveLatestVerifiedCommitment = reactive.NewVariable[bool]()

		directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
			return parentVerified && !verified
		}, parentVerified, commitment.verified)
	)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		parentVerified.InheritFrom(parent.verified)
		parentAboveLatestVerifiedCommitment.InheritFrom(parent.isAboveLatestVerifiedIndex)
	})

	return reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment)
}

// triggerEventIfCommitmentBelowThreshold triggers the given event if the given commitment is below the given threshold.
func triggerEventIfCommitmentBelowThreshold(event func(*Commitment) reactive.Event, commitment *Commitment, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
	// only monitor the threshold after the parent event was triggered (minimize listeners to same threshold)
	event(commitment.parent.Get()).OnTrigger(func() {
		// since events only trigger once, we unsubscribe from the threshold after the trigger condition is met
		chainThreshold(commitment.chain.Get()).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
			event(commitment).Trigger()
		}, func(_, slotIndex iotago.SlotIndex) bool {
			return commitment.Index() < slotIndex
		})
	})
}
