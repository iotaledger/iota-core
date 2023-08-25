package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chain struct {
	root reactive.Variable[*CommitmentMetadata]

	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *CommitmentMetadata]

	latestCommitmentIndex reactive.Variable[iotago.SlotIndex]

	latestVerifiedCommitmentIndex reactive.Variable[iotago.SlotIndex]

	// syncThreshold defines an upper bound for the range of slots that are being fed to the engine as part of the sync
	// process (sync from past to present preventing the engine from running out of memory).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines a lower bound for where the warp sync process starts (to not requests slots that we are
	// about to commit ourselves once we are in sync).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]

	cumulativeWeight reactive.Variable[uint64]

	verifiedCumulativeWeight reactive.Variable[uint64]

	evicted reactive.Event
}

func NewChain(rootCommitment *CommitmentMetadata) *Chain {
	c := &Chain{
		commitments:                   shrinkingmap.New[iotago.SlotIndex, *CommitmentMetadata](),
		root:                          reactive.NewVariable[*CommitmentMetadata]().Init(rootCommitment),
		latestCommitmentIndex:         reactive.NewVariable[iotago.SlotIndex](),
		latestVerifiedCommitmentIndex: reactive.NewVariable[iotago.SlotIndex](),
		evicted:                       reactive.NewEvent(),
	}

	rootCommitment.Chain().Set(c)

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](ComputeSyncThreshold, c.latestVerifiedCommitmentIndex)
	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](ComputeWarpSyncThreshold, c.latestCommitmentIndex)
	c.cumulativeWeight = reactive.NewDerivedVariable[uint64](c.computeCumulativeWeightOfSlot, c.latestCommitmentIndex)
	c.verifiedCumulativeWeight = reactive.NewDerivedVariable[uint64](c.computeCumulativeWeightOfSlot, c.latestVerifiedCommitmentIndex)

	return c
}

func (c *Chain) Root() reactive.Variable[*CommitmentMetadata] {
	return c.root
}

func (c *Chain) Commitment(index iotago.SlotIndex) (commitment *CommitmentMetadata, exists bool) {
	parentChain := func(c *Chain) *Chain {
		if root := c.root.Get(); root != nil {
			if parent := root.Parent().Get(); parent != nil {
				return parent.Chain().Get()
			}
		}

		return nil
	}

	for currentChain := c; currentChain != nil; currentChain = parentChain(currentChain) {
		if root := currentChain.Root().Get(); root != nil && index >= root.Index() {
			return currentChain.commitments.Get(index)
		}
	}

	return nil, false
}

func (c *Chain) LatestCommitmentIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestCommitmentIndex
}

func (c *Chain) LatestVerifiedCommitmentIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestVerifiedCommitmentIndex
}

func (c *Chain) SyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.syncThreshold
}

func (c *Chain) WarpSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.warpSyncThreshold
}

func (c *Chain) CumulativeWeight() reactive.Variable[uint64] {
	return c.cumulativeWeight
}

func (c *Chain) registerCommitment(commitment *CommitmentMetadata) {
	c.commitments.Set(commitment.Index(), commitment)

	c.latestCommitmentIndex.Compute(func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return lo.Cond(latestCommitmentIndex > commitment.Index(), latestCommitmentIndex, commitment.Index())
	})

	unsubscribe := commitment.verified.OnTrigger(func() {
		c.latestVerifiedCommitmentIndex.Compute(func(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
			return lo.Cond(latestVerifiedCommitmentIndex > commitment.Index(), latestVerifiedCommitmentIndex, commitment.Index())
		})
	})

	triggerIfSwitchedChains := func(_, newChain *Chain) bool { return newChain != c }

	commitment.Chain().OnUpdateOnce(func(_, _ *Chain) {
		unsubscribe()

		c.unregisterCommitment(commitment)
	}, triggerIfSwitchedChains)
}

func (c *Chain) unregisterCommitment(commitment *CommitmentMetadata) {
	c.commitments.Delete(commitment.Index())

	c.latestCommitmentIndex.Compute(func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return lo.Cond(commitment.Index() < latestCommitmentIndex, commitment.Index()-1, latestCommitmentIndex)
	})

	c.latestVerifiedCommitmentIndex.Compute(func(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return lo.Cond(commitment.Index() < latestVerifiedCommitmentIndex, commitment.Index()-1, latestVerifiedCommitmentIndex)
	})
}

func (c *Chain) computeCumulativeWeightOfSlot(slotIndex iotago.SlotIndex) uint64 {
	if commitment, exists := c.commitments.Get(slotIndex); exists {
		return commitment.CumulativeWeight()
	}

	return 0
}
