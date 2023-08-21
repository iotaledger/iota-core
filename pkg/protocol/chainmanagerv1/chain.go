package chainmanagerv1

import (
	"fmt"

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
	forkingPoint reactive.Variable[*CommitmentMetadata]

	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *CommitmentMetadata]

	latestCommitmentIndex reactive.Variable[iotago.SlotIndex]

	latestVerifiedCommitmentIndex reactive.Variable[iotago.SlotIndex]

	evicted reactive.Event

	// syncThreshold defines an upper bound for the range of slots that are being fed to the engine as part of the sync
	// process (sync from past to present preventing the engine from running out of memory).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines a lower bound for where the warp sync process starts (to not requests slots that we are
	// about to commit ourselves once we are in sync).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]

	cumulativeWeight reactive.Variable[uint64]
}

func NewChain(forkingPoint *CommitmentMetadata) *Chain {
	c := &Chain{
		forkingPoint:                  reactive.NewVariable[*CommitmentMetadata](),
		commitments:                   shrinkingmap.New[iotago.SlotIndex, *CommitmentMetadata](),
		evicted:                       reactive.NewEvent(),
		latestCommitmentIndex:         reactive.NewVariable[iotago.SlotIndex](),
		latestVerifiedCommitmentIndex: reactive.NewVariable[iotago.SlotIndex](),
	}

	c.forkingPoint.Set(forkingPoint)
	c.latestCommitmentIndex.Set(forkingPoint.Index())

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestVerifiedCommitmentIndex + 1 + SyncWindow
	}, c.latestVerifiedCommitmentIndex)

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestCommitmentIndex - WarpSyncOffset
	}, c.latestCommitmentIndex)

	c.cumulativeWeight = reactive.NewDerivedVariable[uint64](func(latestCommitmentIndex iotago.SlotIndex) uint64 {
		if latestCommitment, exists := c.commitments.Get(latestCommitmentIndex); exists {
			return latestCommitment.CumulativeWeight()
		}

		panic(fmt.Sprintf("latest commitment with index %d does not exist", latestCommitmentIndex))
	}, c.latestCommitmentIndex)

	return c
}

func (c *Chain) ForkingPoint() reactive.Variable[*CommitmentMetadata] {
	return c.forkingPoint
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

		if latestCommitmentIndex > commitment.Index() {
			return latestCommitmentIndex
		}

		return commitment.Index()
	})

	unregistered := reactive.NewEvent()

	unsubscribe := commitment.Chain().OnUpdate(func(_, newValue *Chain) {
		if newValue != c {
			unregistered.Trigger()
		}
	})

	unregistered.OnTrigger(func() {
		go unsubscribe()

		c.latestCommitmentIndex.Compute(func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
			c.commitments.Delete(commitment.Index())

			return lo.Cond(commitment.Index() < latestCommitmentIndex, commitment.Index()-1, latestCommitmentIndex)
		})
	})
}
