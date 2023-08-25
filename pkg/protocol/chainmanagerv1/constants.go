package chainmanagerv1

import iotago "github.com/iotaledger/iota.go/v4"

const (
	// SyncWindow defines the maximum amount of slots that a node requests on top of its latest verified commitment.
	SyncWindow = 20

	// WarpSyncOffset defines how many slots a commitment needs to be behind the latest commitment to be requested by
	// the warp sync process.
	WarpSyncOffset = 1
)

func ComputeWarpSyncThreshold(latestCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
	if WarpSyncOffset > latestCommitmentIndex {
		return 0
	}

	return latestCommitmentIndex - WarpSyncOffset
}

func ComputeSyncThreshold(latestVerifiedCommitmentIndex iotago.SlotIndex) iotago.SlotIndex {
	return latestVerifiedCommitmentIndex + 1 + SyncWindow
}
