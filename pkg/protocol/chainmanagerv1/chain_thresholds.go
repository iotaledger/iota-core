package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ChainThresholds is a reactive component that provides a set of thresholds that are derived from the chain.
type ChainThresholds struct {
	// chain is the chain that this component belongs to.
	chain *Chain

	// latestIndex is the index of the latest CommitmentMetadata object in the chain.
	latestIndex reactive.Variable[iotago.SlotIndex]

	// latestAttestedIndex is the index of the latest attested CommitmentMetadata object in the chain.
	latestAttestedIndex reactive.Variable[iotago.SlotIndex]

	// latestVerifiedIndex is the index of the latest verified CommitmentMetadata object in the chain.
	latestVerifiedIndex reactive.Variable[iotago.SlotIndex]

	// syncThreshold is the upper bound for slots that are being fed to the engine (to prevent memory exhaustion).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines an offset from latest index where the warp sync process starts (we don't request slots
	// that we are about to commit ourselves).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]
}

// NewChainThresholds creates a new ChainThresholds instance.
func NewChainThresholds(chain *Chain) *ChainThresholds {
	c := &ChainThresholds{
		chain:               chain,
		latestIndex:         reactive.NewDerivedVariable[iotago.SlotIndex](zeroValueIfNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatest()),
		latestAttestedIndex: reactive.NewDerivedVariable[iotago.SlotIndex](zeroValueIfNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatestAttested()),
		latestVerifiedIndex: reactive.NewDerivedVariable[iotago.SlotIndex](zeroValueIfNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatestVerified()),
	}

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestIndex - WarpSyncOffset
	}, c.latestIndex)

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestVerifiedIndex + 1 + SyncWindow
	}, c.latestVerifiedIndex)

	return c
}

// Chain returns the chain that this component belongs to.
func (c *ChainThresholds) Chain() *Chain {
	return c.chain
}

// LatestIndex returns the index of the latest CommitmentMetadata object in the chain.
func (c *ChainThresholds) LatestIndex() iotago.SlotIndex {
	return c.latestIndex.Get()
}

// LatestAttestedIndex returns the index of the latest attested CommitmentMetadata object in the chain.
func (c *ChainThresholds) LatestAttestedIndex() iotago.SlotIndex {
	return c.latestAttestedIndex.Get()
}

// LatestVerifiedIndex returns the index of the latest verified CommitmentMetadata object in the chain.
func (c *ChainThresholds) LatestVerifiedIndex() iotago.SlotIndex {
	return c.latestVerifiedIndex.Get()
}

// SyncThreshold returns the upper bound for slots that are being fed to the engine (to prevent memory exhaustion).
func (c *ChainThresholds) SyncThreshold() iotago.SlotIndex {
	return c.syncThreshold.Get()
}

// WarpSyncThreshold returns an offset from latest index where the warp sync process starts (we don't request slots that
// we are about to commit ourselves).
func (c *ChainThresholds) WarpSyncThreshold() iotago.SlotIndex {
	return c.warpSyncThreshold.Get()
}

// ReactiveLatestIndex returns a reactive variable that always contains the index of the latest CommitmentMetadata
// object in the chain.
func (c *ChainThresholds) ReactiveLatestIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestIndex
}

// ReactiveLatestAttestedIndex returns a reactive variable that contains the index of the latest attested
// CommitmentMetadata object in the chain.
func (c *ChainThresholds) ReactiveLatestAttestedIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestAttestedIndex
}

// ReactiveLatestVerifiedIndex returns a reactive variable that contains the index of the latest verified
// CommitmentMetadata object in the chain.
func (c *ChainThresholds) ReactiveLatestVerifiedIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestVerifiedIndex
}

// ReactiveSyncThreshold returns a reactive variable that contains the upper bound for slots that are being fed to the
// engine (to prevent memory exhaustion).
func (c *ChainThresholds) ReactiveSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.syncThreshold
}

// ReactiveWarpSyncThreshold returns a reactive variable that contains an offset from latest index where the warp sync
// process starts (we don't request slots that we are about to commit ourselves).
func (c *ChainThresholds) ReactiveWarpSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.warpSyncThreshold
}
