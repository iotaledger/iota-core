package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type commitmentDispatcherFlags struct {
	isAboveLatestVerifiedCommitment reactive.Variable[bool]
	isBelowSyncThreshold            reactive.Event
	isBelowWarpSyncThreshold        reactive.Event
	inSyncWindow                    reactive.Variable[bool]
	requiresWarpSync                reactive.Variable[bool]
}

func newCommitmentDispatcherFlags(commitment *Commitment, isRoot bool) *commitmentDispatcherFlags {
	c := &commitmentDispatcherFlags{
		isAboveLatestVerifiedCommitment: isAboveLatestVerifiedCommitment(commitment),
		isBelowSyncThreshold:            reactive.NewEvent(),
		isBelowWarpSyncThreshold:        reactive.NewEvent(),
	}

	if isRoot {
		c.isBelowSyncThreshold.Set(true)
		c.isBelowWarpSyncThreshold.Set(true)
	}

	c.inSyncWindow = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, c.isBelowSyncThreshold, c.isAboveLatestVerifiedCommitment)

	c.requiresWarpSync = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.inSyncWindow, c.isBelowWarpSyncThreshold)

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold((*Commitment).IsBelowSyncThreshold, commitment, (*Chain).SyncThreshold)
		triggerEventIfCommitmentBelowThreshold((*Commitment).IsBelowWarpSyncThreshold, commitment, (*Chain).WarpSyncThreshold)
	})

	return c
}

func (c *commitmentDispatcherFlags) IsAboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.isAboveLatestVerifiedCommitment
}

func (c *commitmentDispatcherFlags) IsBelowSyncThreshold() reactive.Event {
	return c.isBelowSyncThreshold
}

func (c *commitmentDispatcherFlags) IsBelowWarpSyncThreshold() reactive.Event {
	return c.isBelowWarpSyncThreshold
}

func (c *commitmentDispatcherFlags) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *commitmentDispatcherFlags) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func isAboveLatestVerifiedCommitment(commitment *Commitment) reactive.Variable[bool] {
	var (
		parentVerified = reactive.NewEvent()

		parentAboveLatestVerifiedCommitment = reactive.NewVariable[bool]()

		directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
			return parentVerified && !verified
		}, parentVerified, commitment.verified)
	)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		parentVerified.InheritFrom(parent.verified)
		parentAboveLatestVerifiedCommitment.InheritFrom(parent.isAboveLatestVerifiedCommitment)
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
