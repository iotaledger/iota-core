package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	// SyncWindow defines the maximum amount of slots that a node requests on top of its latest verified commitment.
	SyncWindow = 20

	// WarpSyncOffset defines how many slots a commitment needs to be behind the latest commitment to be requested by
	// the warp sync process.
	WarpSyncOffset = 1
)

type Chain struct {
	manager *ChainManager

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

	evicted reactive.Event
}

func NewChain(root *CommitmentMetadata, manager *ChainManager) *Chain {
	c := &Chain{
		manager:                       manager,
		commitments:                   shrinkingmap.New[iotago.SlotIndex, *CommitmentMetadata](),
		root:                          reactive.NewVariable[*CommitmentMetadata]().Init(root),
		latestCommitmentIndex:         reactive.NewVariable[iotago.SlotIndex](),
		latestVerifiedCommitmentIndex: reactive.NewVariable[iotago.SlotIndex](),
		evicted:                       reactive.NewEvent(),
	}

	root.Chain().Set(c)

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestVerifiedCommitmentIndex + 1 + SyncWindow
	}, c.latestVerifiedCommitmentIndex)

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestCommitmentIndex - WarpSyncOffset
	}, c.latestCommitmentIndex)

	c.cumulativeWeight = reactive.NewDerivedVariable[uint64](func(latestCommitmentIndex iotago.SlotIndex) uint64 {
		return lo.Return1(c.commitments.Get(latestCommitmentIndex)).CumulativeWeight()
	}, c.latestCommitmentIndex)

	return c
}

func (c *Chain) Root() reactive.Variable[*CommitmentMetadata] {
	return c.root
}

func (c *Chain) ParentChain() *Chain {
	if root := c.root.Get(); root != nil {
		if parent := root.Parent().Get(); parent != nil {
			return parent.Chain().Get()
		}
	}

	return nil
}

func (c *Chain) Commitment(index iotago.SlotIndex) (commitment *CommitmentMetadata, exists bool) {
	for currentChain := c; currentChain != nil; currentChain = currentChain.ParentChain() {
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

func (c *Chain) registerCommitment(commitment *CommitmentMetadata) {
	c.latestCommitmentIndex.Compute(func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		c.commitments.Set(commitment.Index(), commitment)

		return lo.Cond(latestCommitmentIndex > commitment.Index(), latestCommitmentIndex, commitment.Index())
	})

	commitment.Chain().OnUpdateOnce(func(_, _ *Chain) {
		c.unregisterCommitment(commitment)
	}, func(_, newValue *Chain) bool {
		return newValue != c
	})
}

func (c *Chain) unregisterCommitment(commitment *CommitmentMetadata) iotago.SlotIndex {
	return c.latestCommitmentIndex.Compute(func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		c.commitments.Delete(commitment.Index())

		return lo.Cond(commitment.Index() < latestCommitmentIndex, commitment.Index()-1, latestCommitmentIndex)
	})
}
