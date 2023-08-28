package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/model"
)

type Commitment struct {
	*model.Commitment

	isSolid  reactive.Event
	attested reactive.Event
	verified reactive.Event
	evicted  reactive.Event

	*commitmentChainProperties
	*commitmentThresholds

	inSyncWindow     reactive.Variable[bool]
	requiresWarpSync reactive.Variable[bool]
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment: commitment,

		isSolid:  reactive.NewEvent(),
		attested: reactive.NewEvent(),
		verified: reactive.NewEvent(),
		evicted:  reactive.NewEvent(),
	}

	c.commitmentChainProperties = newCommitmentChainProperties(c)
	c.commitmentThresholds = newCommitmentThresholds(c)

	c.inSyncWindow = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, c.isBelowSyncThreshold, c.isAboveLatestVerifiedIndex)

	c.requiresWarpSync = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.inSyncWindow, c.isBelowWarpSyncThreshold)

	return c
}

func NewRootCommitment(commitment *model.Commitment) *Commitment {
	commitmentMetadata := NewCommitment(commitment)
	commitmentMetadata.IsSolidEvent().Trigger()
	commitmentMetadata.Verified().Trigger()
	commitmentMetadata.IsBelowSyncThresholdEvent().Trigger()
	commitmentMetadata.IsBelowWarpSyncThresholdEvent().Trigger()
	commitmentMetadata.IsBelowLatestVerifiedIndexEvent().Trigger()
	commitmentMetadata.Evicted().Trigger()

	return commitmentMetadata
}

func (c *Commitment) IsSolid() bool {
	return c.isSolid.WasTriggered()
}

func (c *Commitment) IsSolidEvent() reactive.Event {
	return c.isSolid
}

func (c *Commitment) Attested() reactive.Event {
	return c.attested
}

func (c *Commitment) Verified() reactive.Event {
	return c.verified
}

func (c *Commitment) Evicted() reactive.Event {
	return c.evicted
}

func (c *Commitment) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *Commitment) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func (c *Commitment) ChainSuccessor() reactive.Variable[*Commitment] {
	return c.successor
}

// Max compares the given commitment with the current one and returns the one with the higher index.
func (c *Commitment) Max(latestCommitment *Commitment) *Commitment {
	if c == nil || latestCommitment != nil && latestCommitment.Index() >= c.Index() {
		return latestCommitment
	}

	return c
}
