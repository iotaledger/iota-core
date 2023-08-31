package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type aboveLatestVerifiedCommitment struct {
	reactive.Variable[bool]

	parentVerified reactive.Event

	parentAboveLatestVerifiedCommitment reactive.Variable[bool]

	directlyAboveLatestVerifiedCommitment reactive.Variable[bool]
}

func newAboveLatestVerifiedCommitment(commitment *Commitment) *aboveLatestVerifiedCommitment {
	a := &aboveLatestVerifiedCommitment{
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
		a.parentAboveLatestVerifiedCommitment.InheritFrom(parent.inSyncWindow.isAboveLatestVerifiedCommitment)
	})

	return a
}
