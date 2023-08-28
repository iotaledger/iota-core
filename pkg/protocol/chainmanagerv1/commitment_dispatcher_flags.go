package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type commitmentDispatcherFlags struct {
	inSyncWindow     reactive.Variable[bool]
	requiresWarpSync reactive.Variable[bool]
}

func newCommitmentDispatcherFlags(commitment *Commitment) *commitmentDispatcherFlags {
	c := &commitmentDispatcherFlags{}

	c.inSyncWindow = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, commitment.isBelowSyncThreshold, commitment.isAboveLatestVerifiedIndex)

	c.requiresWarpSync = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.inSyncWindow, commitment.isBelowWarpSyncThreshold)

	return c
}

func (c *commitmentDispatcherFlags) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *commitmentDispatcherFlags) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}
