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
	defer triggerBelowThresholdEventsIfNecessary(commitment)

	return &commitmentThresholds{
		isAboveLatestVerifiedIndex: isAboveLatestVerifiedIndex(commitment),
		isBelowLatestVerifiedIndex: reactive.NewEvent(),
		isBelowSyncThreshold:       reactive.NewEvent(),
		isBelowWarpSyncThreshold:   reactive.NewEvent(),
	}
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

	commitment.ParentVariable().OnUpdateOnce(func(_, parent *Commitment) {
		parentVerified.InheritFrom(parent.verified)
		parentAboveLatestVerifiedCommitment.InheritFrom(parent.isAboveLatestVerifiedIndex)
	})

	return reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment)
}

func triggerBelowThresholdEventsIfNecessary(commitment *Commitment) func() {
	return commitment.ParentVariable().OnUpdate(func(_, parent *Commitment) {
		// we only monitor the threshold after the corresponding parent event was triggered (to minimize the amount of
		// elements that listen to updates of the same threshold - it spreads monotonically).
		triggerIfBelowThreshold := func(event func(*Commitment) reactive.Event, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
			event(parent).OnTrigger(func() {
				chainThreshold(commitment.Chain()).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
					event(commitment).Trigger()
				}, func(_ iotago.SlotIndex, slotIndex iotago.SlotIndex) bool {
					return commitment.Index() < slotIndex
				})
			})
		}

		triggerIfBelowThreshold((*Commitment).IsBelowLatestVerifiedIndexEvent, (*Chain).LatestVerifiedIndex)
		triggerIfBelowThreshold((*Commitment).IsBelowSyncThresholdEvent, (*Chain).SyncThreshold)
		triggerIfBelowThreshold((*Commitment).IsBelowWarpSyncThresholdEvent, (*Chain).WarpSyncThreshold)
	})
}
