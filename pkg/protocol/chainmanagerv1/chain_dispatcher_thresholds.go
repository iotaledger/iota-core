package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	iotago "github.com/iotaledger/iota.go/v4"
)

// chainDispatcherThresholds is a reactive component that provides a set of thresholds that are derived from the chain.
type chainDispatcherThresholds struct {
	// syncThreshold is the upper bound for slots that are being fed to the engine (to prevent memory exhaustion).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines an offset from latest index where the warp sync process starts (we don't request slots
	// that we are about to commit ourselves).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]
}

// newChainDispatcherThresholds creates a new chainDispatcherThresholds instance.
func newChainDispatcherThresholds(chain *Chain) *chainDispatcherThresholds {
	return &chainDispatcherThresholds{
		warpSyncThreshold: reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitment *Commitment) iotago.SlotIndex {
			if latestCommitment == nil || latestCommitment.Index() < WarpSyncOffset {
				return 0
			}

			return latestCommitment.Index() - WarpSyncOffset
		}, chain.latestCommitment),

		syncThreshold: reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitment *Commitment) iotago.SlotIndex {
			if latestVerifiedCommitment == nil {
				return SyncWindow + 1
			}

			return latestVerifiedCommitment.Index() + SyncWindow + 1
		}, chain.latestVerifiedCommitment),
	}
}

// SyncThreshold returns a reactive variable that contains the upper bound for slots that are being fed to the
// engine (to prevent memory exhaustion).
func (c *chainDispatcherThresholds) SyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.syncThreshold
}

// WarpSyncThreshold returns a reactive variable that contains an offset from latest index where the warp sync
// process starts (we don't request slots that we are about to commit ourselves).
func (c *chainDispatcherThresholds) WarpSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.warpSyncThreshold
}
