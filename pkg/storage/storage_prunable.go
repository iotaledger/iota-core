package storage

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Storage) RewardsForEpoch(epoch iotago.EpochIndex) kvstore.KVStore {
	return s.prunable.RewardsForEpoch(epoch)
}

func (s *Storage) Rewards() *epochstore.EpochKVStore {
	return s.prunable.Rewards()
}

func (s *Storage) PoolStats() *epochstore.Store[*model.PoolsStats] {
	return s.prunable.PoolStats()
}

func (s *Storage) DecidedUpgradeSignals() *epochstore.Store[model.VersionAndHash] {
	return s.prunable.DecidedUpgradeSignals()
}

func (s *Storage) Committee() *epochstore.Store[*account.Accounts] {
	return s.prunable.Committee()
}

func (s *Storage) Blocks(slot iotago.SlotIndex) *slotstore.Blocks {
	return s.prunable.Blocks(slot)
}

func (s *Storage) RootBlocks(slot iotago.SlotIndex) *slotstore.Store[iotago.BlockID, iotago.CommitmentID] {
	return s.prunable.RootBlocks(slot)
}

func (s *Storage) Attestations(slot iotago.SlotIndex) kvstore.KVStore {
	return s.prunable.Attestations(slot)
}

func (s *Storage) AccountDiffs(slot iotago.SlotIndex) *slotstore.AccountDiffs {
	return s.prunable.AccountDiffs(slot)
}

func (s *Storage) PerformanceFactors(slot iotago.SlotIndex) *slotstore.Store[iotago.AccountID, uint64] {
	return s.prunable.PerformanceFactors(slot)
}

func (s *Storage) UpgradeSignals(slot iotago.SlotIndex) *slotstore.Store[account.SeatIndex, *model.SignaledBlock] {
	return s.prunable.UpgradeSignals(slot)
}

func (s *Storage) Roots(slot iotago.SlotIndex) *slotstore.Store[iotago.CommitmentID, *iotago.Roots] {
	return s.prunable.Roots(slot)
}

func (s *Storage) Retainer(slot iotago.SlotIndex) *slotstore.Retainer {
	return s.prunable.Retainer(slot)
}

func (s *Storage) LastPrunedEpoch() (index iotago.EpochIndex, hasPruned bool) {
	return s.prunable.LastPrunedEpoch()
}

func (s *Storage) RestoreFromDisk() {
	s.prunable.RestoreFromDisk()
}
