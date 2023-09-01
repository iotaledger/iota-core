package protocol

import "github.com/iotaledger/hive.go/ds/reactive"

type requestBlocks struct {
	reactive.Variable[bool]

	isBelowWarpSyncThreshold reactive.Event
}

func newRequestBlocks(commitment *Commitment, isRoot bool) *requestBlocks {
	r := &requestBlocks{
		isBelowWarpSyncThreshold: reactive.NewEvent(),
	}

	if isRoot {
		r.isBelowWarpSyncThreshold.Set(true)
	}

	r.Variable = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, commitment.inSyncRange, r.isBelowWarpSyncThreshold)

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.requestBlocks.isBelowWarpSyncThreshold
		}, commitment, (*Chain).WarpSyncThreshold)
	})

	return r
}
