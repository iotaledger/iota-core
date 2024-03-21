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

	// IsFinalizationDelayed returns bool indicating if the finalization is delayed
	// (latest committed slot - latest finalized slot > max committable age).
	IsFinalizationDelayed() bool

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

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}

type SyncStatus struct {
	NodeBootstrapped       bool
	NodeSynced             bool
	FinalizationDelayed    bool
	LastAcceptedBlockSlot  iotago.SlotIndex
	LastConfirmedBlockSlot iotago.SlotIndex
	LatestCommitment       *model.Commitment
	LatestFinalizedSlot    iotago.SlotIndex
	LastPrunedEpoch        iotago.EpochIndex
	HasPruned              bool
}
