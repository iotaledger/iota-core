package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainThresholds struct {
	latestIndex reactive.Variable[iotago.SlotIndex]

	latestAttestedIndex reactive.Variable[iotago.SlotIndex]

	latestVerifiedIndex reactive.Variable[iotago.SlotIndex]

	// syncThreshold defines an upper bound for the range of slots that are being fed to the engine as part of the sync
	// process (sync from past to present preventing the engine from running out of memory).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines a lower bound for where the warp sync process starts (to not requests slots that we are
	// about to commit ourselves once we are in sync).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]
}

func NewChainThresholds(chain *Chain) *ChainThresholds {
	c := &ChainThresholds{
		latestIndex:         reactive.NewDerivedVariable[iotago.SlotIndex](ignoreNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatest()),
		latestAttestedIndex: reactive.NewDerivedVariable[iotago.SlotIndex](ignoreNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatestAttested()),
		latestVerifiedIndex: reactive.NewDerivedVariable[iotago.SlotIndex](ignoreNil((*CommitmentMetadata).Index), chain.Commitments().ReactiveLatestVerified()),
	}

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestIndex - WarpSyncOffset
	}, c.latestIndex)

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedIndex iotago.SlotIndex) iotago.SlotIndex {
		return latestVerifiedIndex + 1 + SyncWindow
	}, c.latestVerifiedIndex)

	return c
}

func (c *ChainThresholds) LatestIndex() iotago.SlotIndex {
	return c.latestIndex.Get()
}

func (c *ChainThresholds) LatestAttestedIndex() iotago.SlotIndex {
	return c.latestAttestedIndex.Get()
}

func (c *ChainThresholds) LatestVerifiedIndex() iotago.SlotIndex {
	return c.latestVerifiedIndex.Get()
}

func (c *ChainThresholds) SyncThreshold() iotago.SlotIndex {
	return c.syncThreshold.Get()
}

func (c *ChainThresholds) WarpSyncThreshold() iotago.SlotIndex {
	return c.warpSyncThreshold.Get()
}

func (c *ChainThresholds) ReactiveLatestIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestIndex
}

func (c *ChainThresholds) ReactiveLatestAttestedIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestAttestedIndex
}

func (c *ChainThresholds) ReactiveLatestVerifiedIndex() reactive.Variable[iotago.SlotIndex] {
	return c.latestVerifiedIndex
}

func (c *ChainThresholds) ReactiveSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.syncThreshold
}

func (c *ChainThresholds) ReactiveWarpSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.warpSyncThreshold
}
