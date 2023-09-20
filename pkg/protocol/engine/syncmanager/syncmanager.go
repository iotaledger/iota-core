package syncmanager

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SyncManager interface {
	// SyncStatus returns the sync status of a node.
	SyncStatus() *SyncStatus

	// IsBootstrapped returns bool indicating if a node is bootstrapped.
	IsBootstrapped() bool

	// IsNodeSynced returns bool indicating if a node is synced.
	IsNodeSynced() bool

	// LastAcceptedBlockSlot returns the slot of the latest accepted block.
	LastAcceptedBlockSlot() iotago.SlotIndex

	// LastConfirmedBlockSlot returns slot of the latest confirmed block.
	LastConfirmedBlockSlot() iotago.SlotIndex

	// LatestCommitment returns the latest commitment.
	LatestCommitment() *model.Commitment

	// LatestFinalizedSlot returns the latest finalized slot index.
	LatestFinalizedSlot() iotago.SlotIndex

	// LastPrunedEpoch returns the last pruned epoch index.
	LastPrunedEpoch() (iotago.EpochIndex, bool)

	// Shutdown shuts down the SyncManager.
	Shutdown()

	module.Interface
}

type SyncStatus struct {
	NodeSynced             bool
	LastAcceptedBlockSlot  iotago.SlotIndex
	LastConfirmedBlockSlot iotago.SlotIndex
	LatestCommitment       *model.Commitment
	LatestFinalizedSlot    iotago.SlotIndex
	LastPrunedEpoch        iotago.EpochIndex
	HasPruned              bool
}
