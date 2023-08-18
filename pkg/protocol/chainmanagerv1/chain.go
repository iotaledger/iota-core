package chainmanagerv1

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	// SyncWindow defines the maximum amount of slots that a node requests on top of its latest verified commitment.
	SyncWindow = 20

	// WarpSyncOffset defines how many slots a commitment needs to be behind the latest commitment to be requested by
	// the warp sync process.
	WarpSyncOffset = 2
)

type Chain struct {
	latestCommitmentIndex         reactive.Variable[iotago.SlotIndex]
	latestVerifiedCommitmentIndex reactive.Variable[iotago.SlotIndex]

	evicted reactive.Event

	// syncThreshold defines an upper bound for the range of slots that are being fed to the engine as part of the sync
	// process (sync from past to present preventing the engine from running out of memory).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines a lower bound for where the warp sync process starts (to not requests slots that we are
	// about to commit ourselves once we are in sync).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]
}

func NewChain() *Chain {
	c := &Chain{
		evicted:                       reactive.NewEvent(),
		latestCommitmentIndex:         reactive.NewVariable[iotago.SlotIndex](),
		latestVerifiedCommitmentIndex: reactive.NewVariable[iotago.SlotIndex](),
	}

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestVerifiedCommitmentIndex + 1 + SyncWindow
	}, c.latestVerifiedCommitmentIndex)

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestCommitmentIndex - WarpSyncOffset
	}, c.latestCommitmentIndex)

	return c
}

func (c *Chain) RegisterCommitment(commitment *CommitmentMetadata) {
	fmt.Println("RegisterCommitment", commitment.ID(), "as index", commitment.Index())
}

func (c *Chain) UnregisterCommitment(commitment *CommitmentMetadata) {
	fmt.Println("UnregisterCommitment", commitment.ID(), "as index", commitment.Index())
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
