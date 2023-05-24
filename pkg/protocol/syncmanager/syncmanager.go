package syncmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SyncManager interface {
	// SyncStatus returns the sync status of a node.
	SyncStatus() *SyncStatus

	// IsNodeSynced returns bool indicating if a node is synced.
	IsNodeSynced() bool

	// LastAcceptedBlock returns the latest accepted block ID.
	LastAcceptedBlock() iotago.BlockID

	// LastConfirmedBlock returns the latest confirmed block ID.
	LastConfirmedBlock() iotago.BlockID

	// FinalizedSlot returns the latest finalized slot index.
	FinalizedSlot() iotago.SlotIndex

	// LatestCommittedSlot returns the latest committed slot index.
	LatestCommittedSlot() iotago.SlotIndex

	// Shutdown shuts down the SyncManager.
	Shutdown()

	module.Interface
}

type SyncStatus struct {
	NodeSynced           bool
	LastAcceptedBlockID  iotago.BlockID
	LastConfirmedBlockID iotago.BlockID
	FinalizedSlot        iotago.SlotIndex
	LatestCommittedSlot  iotago.SlotIndex
}
