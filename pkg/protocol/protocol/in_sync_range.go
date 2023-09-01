package protocol

import "github.com/iotaledger/hive.go/ds/reactive"

type inSyncRange struct {
	reactive.Variable[bool]

	aboveLatestVerifiedCommitment *aboveLatestVerifiedCommitmentVariable
	isBelowSyncThreshold          reactive.Event
}

func newInSyncRange(commitment *Commitment, isRoot bool) *inSyncRange {
	i := &inSyncRange{
		aboveLatestVerifiedCommitment: newAboveLatestVerifiedCommitmentVariable(commitment),
		isBelowSyncThreshold:          reactive.NewEvent(),
	}

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.inSyncRange.isBelowSyncThreshold
		}, commitment, (*Chain).SyncThreshold)
	})

	if isRoot {
		i.isBelowSyncThreshold.Set(true)
	}

	i.Variable = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, i.isBelowSyncThreshold, newAboveLatestVerifiedCommitmentVariable(commitment))

	return i
}

type aboveLatestVerifiedCommitmentVariable struct {
	reactive.Variable[bool]

	parentVerified reactive.Event

	parentAboveLatestVerifiedCommitment reactive.Variable[bool]

	directlyAboveLatestVerifiedCommitment reactive.Variable[bool]
}

func newAboveLatestVerifiedCommitmentVariable(commitment *Commitment) *aboveLatestVerifiedCommitmentVariable {
	a := &aboveLatestVerifiedCommitmentVariable{
		parentVerified:                      reactive.NewEvent(),
		parentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
	}

	a.directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
		return parentVerified && !verified
	}, a.parentVerified, commitment.verified)

	a.Variable = reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, a.directlyAboveLatestVerifiedCommitment, a.parentAboveLatestVerifiedCommitment)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		a.parentVerified.InheritFrom(parent.verified)
		a.parentAboveLatestVerifiedCommitment.InheritFrom(parent.inSyncRange.aboveLatestVerifiedCommitment)
	})

	return a
}
