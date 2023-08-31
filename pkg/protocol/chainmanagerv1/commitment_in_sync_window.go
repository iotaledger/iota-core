package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type inSyncWindow struct {
	reactive.Variable[bool]

	isAboveLatestVerifiedCommitment *aboveLatestVerifiedCommitment
	isBelowSyncThreshold            reactive.Event
}

func newInSyncWindow(commitment *Commitment, isRoot bool) *inSyncWindow {
	i := &inSyncWindow{
		isAboveLatestVerifiedCommitment: newAboveLatestVerifiedCommitment(commitment),
		isBelowSyncThreshold:            reactive.NewEvent(),
	}

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.inSyncWindow.isBelowSyncThreshold
		}, commitment, (*Chain).SyncThreshold)
	})

	if isRoot {
		i.isBelowSyncThreshold.Set(true)
	}

	i.Variable = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, i.isBelowSyncThreshold, newAboveLatestVerifiedCommitment(commitment))

	return i
}
