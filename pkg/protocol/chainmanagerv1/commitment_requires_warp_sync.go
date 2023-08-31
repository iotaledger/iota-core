package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

type requiresWarpSync struct {
	reactive.Variable[bool]

	isBelowWarpSyncThreshold reactive.Event
}

func newRequiresWarpSync(commitment *Commitment, isRoot bool) *requiresWarpSync {
	r := &requiresWarpSync{
		isBelowWarpSyncThreshold: reactive.NewEvent(),
	}

	if isRoot {
		r.isBelowWarpSyncThreshold.Set(true)
	}

	r.Variable = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, commitment.inSyncWindow, r.isBelowWarpSyncThreshold)

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.requiresWarpSync.isBelowWarpSyncThreshold
		}, commitment, (*Chain).WarpSyncThreshold)
	})

	return r
}
