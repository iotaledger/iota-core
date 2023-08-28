package chainmanagerv1

import (
	"github.com/iotaledger/iota-core/pkg/model"
)

type Commitment struct {
	*model.Commitment

	*commitmentDAG
	*commitmentFlags
	*commitmentThresholds
	*commitmentDispatcherFlags
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{Commitment: commitment}

	c.commitmentDAG = newCommitmentDAG(c)
	c.commitmentFlags = newCommitmentFlags(c)
	c.commitmentThresholds = newCommitmentThresholds(c)
	c.commitmentDispatcherFlags = newCommitmentDispatcherFlags(c)

	return c
}

func NewRootCommitment(commitment *model.Commitment) *Commitment {
	commitmentMetadata := NewCommitment(commitment)

	commitmentMetadata.isSolid.Set(true)
	commitmentMetadata.isVerified.Set(true)
	commitmentMetadata.isBelowSyncThreshold.Set(true)
	commitmentMetadata.isBelowWarpSyncThreshold.Set(true)
	commitmentMetadata.isBelowLatestVerifiedIndex.Set(true)
	commitmentMetadata.isEvicted.Set(false)

	return commitmentMetadata
}

// max compares the commitment with the given other commitment and returns the one with the higher index.
func (c *Commitment) max(other *Commitment) *Commitment {
	if c == nil || other != nil && other.Index() >= c.Index() {
		return other
	}

	return c
}
