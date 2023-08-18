package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

func (c *Commitment) Chain() reactive.Variable[*Chain] {
	return c.chain
}

func (c *Commitment) Solid() reactive.Event {
	return c.solid
}

func (c *Commitment) Verified() reactive.Event {
	return c.verified
}

func (c *Commitment) ParentVerified() reactive.Event {
	return c.parentVerified
}

func (c *Commitment) BelowSyncThreshold() reactive.Event {
	return c.belowSyncThreshold
}

func (c *Commitment) BelowWarpSyncThreshold() reactive.Event {
	return c.belowWarpSyncThreshold
}

func (c *Commitment) BelowLatestVerifiedCommitment() reactive.Event {
	return c.belowLatestVerifiedCommitment
}

func (c *Commitment) Evicted() reactive.Event {
	return c.evicted
}

func (c *Commitment) ParentAboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.parentAboveLatestVerifiedCommitment
}

func (c *Commitment) AboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.aboveLatestVerifiedCommitment
}

func (c *Commitment) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *Commitment) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func (c *Commitment) ChainSuccessor() reactive.Variable[*Commitment] {
	return c.chainSuccessor
}
