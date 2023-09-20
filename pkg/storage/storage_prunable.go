package storage

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Storage) RewardsForEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
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

func (s *Storage) Blocks(slot iotago.SlotIndex) (*slotstore.Blocks, error) {
	return s.prunable.Blocks(slot)
}

func (s *Storage) RootBlocks(slot iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error) {
	return s.prunable.RootBlocks(slot)
}

func (s *Storage) Attestations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	return s.prunable.Attestations(slot)
}

func (s *Storage) AccountDiffs(slot iotago.SlotIndex) (*slotstore.AccountDiffs, error) {
	return s.prunable.AccountDiffs(slot)
}

func (s *Storage) PerformanceFactors(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, uint64], error) {
	return s.prunable.PerformanceFactors(slot)
}

func (s *Storage) UpgradeSignals(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error) {
	return s.prunable.UpgradeSignals(slot)
}

func (s *Storage) Roots(slot iotago.SlotIndex) (*slotstore.Store[iotago.CommitmentID, *iotago.Roots], error) {
	return s.prunable.Roots(slot)
}

func (s *Storage) Retainer(slot iotago.SlotIndex) (*slotstore.Retainer, error) {
	return s.prunable.Retainer(slot)
}

func (s *Storage) RestoreFromDisk() {
	s.pruningLock.Lock()
	defer s.pruningLock.Unlock()

	lastPrunedEpoch := s.prunable.RestoreFromDisk()
	// Return if it's epoch 0, leave the lastPrunedEpoch at the default value, indicating epoch 0 is not pruned yet.
	if lastPrunedEpoch == 0 {
		return
	}

	s.lastPrunedEpoch.MarkEvicted(lastPrunedEpoch)
}

func (s *Storage) RollbackPrunable(targetIndex iotago.SlotIndex) error {
	return s.prunable.Rollback(targetIndex)
}
