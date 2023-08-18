package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

func (c *CommitmentMetadata) Chain() reactive.Variable[*Chain] {
	return c.chain
}

func (c *CommitmentMetadata) Solid() reactive.Event {
	return c.solid
}

func (c *CommitmentMetadata) Verified() reactive.Event {
	return c.verified
}

func (c *CommitmentMetadata) ParentVerified() reactive.Event {
	return c.parentVerified
}

func (c *CommitmentMetadata) BelowSyncThreshold() reactive.Event {
	return c.belowSyncThreshold
}

func (c *CommitmentMetadata) BelowWarpSyncThreshold() reactive.Event {
	return c.belowWarpSyncThreshold
}

func (c *CommitmentMetadata) BelowLatestVerifiedCommitment() reactive.Event {
	return c.belowLatestVerifiedCommitment
}

func (c *CommitmentMetadata) Evicted() reactive.Event {
	return c.evicted
}

func (c *CommitmentMetadata) ParentAboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.parentAboveLatestVerifiedCommitment
}

func (c *CommitmentMetadata) AboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.aboveLatestVerifiedCommitment
}

func (c *CommitmentMetadata) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *CommitmentMetadata) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func (c *CommitmentMetadata) ChainSuccessor() reactive.Variable[*CommitmentMetadata] {
	return c.chainSuccessor
}
