package testsuite

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertPrunedUntil(expectedStorage *types.Tuple[int, bool],
	expectedDecidedUpgrades *types.Tuple[int, bool],
	expectedPoolStats *types.Tuple[int, bool],
	expectedCommittee *types.Tuple[int, bool],
	expectedRewards *types.Tuple[int, bool], nodes ...*mock.Node) {

	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if err := t.assertPrunedUntil(node.Protocol.MainEngineInstance().Storage, expectedStorage, expectedDecidedUpgrades, expectedPoolStats, expectedCommittee, expectedRewards); err != nil {
				return ierrors.Wrapf(err, "AssertPrunedSlot: %s", node.Name)
			}

			return nil
		})
	}
}

func (t *TestSuite) assertPrunedUntil(storageInstance *storage.Storage,
	expectedStorage *types.Tuple[int, bool],
	expectedDecidedUpgrades *types.Tuple[int, bool],
	expectedPoolStats *types.Tuple[int, bool],
	expectedCommittee *types.Tuple[int, bool],
	expectedRewards *types.Tuple[int, bool]) error {

	if err := t.assertPrunedState(expectedStorage, storageInstance.LastPrunedEpoch, "prunable"); err != nil {
		return err
	}
	if err := t.assertPrunedState(expectedPoolStats, storageInstance.PoolStats().LastPrunedEpoch, "pool stats"); err != nil {
		return err
	}
	if err := t.assertPrunedState(expectedDecidedUpgrades, storageInstance.DecidedUpgradeSignals().LastPrunedEpoch, "decided upgrades"); err != nil {
		return err
	}
	if err := t.assertPrunedState(expectedCommittee, storageInstance.Committee().LastPrunedEpoch, "committee"); err != nil {
		return err
	}
	if err := t.assertPrunedState(expectedRewards, storageInstance.Rewards().LastPrunedEpoch, "rewards"); err != nil {
		return err
	}

	// Check that things are actually pruned and the correct error is returned.
	if expectedStorage.B {
		for epoch := iotago.EpochIndex(0); epoch <= iotago.EpochIndex(expectedStorage.A); epoch++ {
			if err := t.assertPrunableSlotStoragesPruned(storageInstance, epoch); err != nil {
				return err
			}
		}
	}

	if expectedDecidedUpgrades.B {
		for epoch := iotago.EpochIndex(0); epoch <= iotago.EpochIndex(expectedDecidedUpgrades.A); epoch++ {
			if err := assertPrunableEpochStoragesPruned(t, storageInstance.DecidedUpgradeSignals(), epoch); err != nil {
				return err
			}
		}
	}

	if expectedPoolStats.B {
		for epoch := iotago.EpochIndex(0); epoch <= iotago.EpochIndex(expectedPoolStats.A); epoch++ {
			if err := assertPrunableEpochStoragesPruned(t, storageInstance.PoolStats(), epoch); err != nil {
				return err
			}
		}
	}

	if expectedCommittee.B {
		for epoch := iotago.EpochIndex(0); epoch <= iotago.EpochIndex(expectedCommittee.A); epoch++ {
			if err := assertPrunableEpochStoragesPruned(t, storageInstance.Committee(), epoch); err != nil {
				return err
			}
		}
	}

	if expectedRewards.B {
		for epoch := iotago.EpochIndex(0); epoch <= iotago.EpochIndex(expectedRewards.A); epoch++ {
			_, err := storageInstance.RewardsForEpoch(epoch)
			if !ierrors.Is(err, database.ErrEpochPruned) {
				return ierrors.Errorf("expected epoch %d to be pruned when calling RewardsForEpoch", epoch)
			}
		}
	}

	return nil
}

func assertPrunableEpochStoragesPruned[V any](t *TestSuite, store *epochstore.Store[V], epoch iotago.EpochIndex) error {
	// Check that all store returns the expected error when trying to access the data.
	_, err := store.Load(epoch)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Load", epoch)
	}

	var empty V
	err = store.Store(epoch, empty)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Store", epoch)
	}

	// Check that the epoch actually has been deleted.
	var seenEpochs []iotago.EpochIndex
	err = store.Stream(func(epoch iotago.EpochIndex, _ V) error {
		seenEpochs = append(seenEpochs, epoch)
		return nil
	})
	if err != nil {
		return ierrors.Errorf("failed to stream epoch store: %w", err)
	}
	if assert.Contains(t.fakeTesting, seenEpochs, epoch) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Stream", epoch)
	}

	seenEpochs = nil
	err = store.StreamBytes(func(bytes []byte, bytes2 []byte) error {
		epochFromBytes, _, err := iotago.EpochIndexFromBytes(bytes)
		if err != nil {
			return ierrors.Wrapf(err, "failed to parse epoch from bytes")
		}
		seenEpochs = append(seenEpochs, epochFromBytes)

		return nil
	})
	if err != nil {
		return ierrors.Errorf("failed to stream epoch store: %w", err)
	}
	if assert.Contains(t.fakeTesting, seenEpochs, epoch) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling StreamBytes", epoch)
	}

	return nil
}

func (t *TestSuite) assertPrunableSlotStoragesPruned(storageInstance *storage.Storage, epoch iotago.EpochIndex) error {
	// Check that the folder for the epoch is deleted.
	if _, err := os.Stat(filepath.Join(storageInstance.Directory(), "prunable", fmt.Sprintf("%d", epoch))); err == nil {
		return ierrors.Errorf("expected folder for epoch %d to be deleted from disk", epoch)
	}

	// Check that all storages return the expected error when trying to access the data.
	endSlot := storageInstance.Settings().APIProvider().APIForEpoch(epoch).TimeProvider().EpochEnd(epoch)

	_, err := storageInstance.Blocks(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Blocks", epoch)
	}

	_, err = storageInstance.RootBlocks(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling RootBlocks", epoch)
	}

	_, err = storageInstance.Attestations(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Attestations", epoch)
	}

	_, err = storageInstance.AccountDiffs(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling AccountDiffs", epoch)
	}

	_, err = storageInstance.ValidatorPerformances(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling ValidatorPerformances", epoch)
	}

	_, err = storageInstance.UpgradeSignals(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling UpgradeSignals", epoch)
	}

	_, err = storageInstance.Roots(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Roots", epoch)
	}

	_, err = storageInstance.Retainer(endSlot)
	if !ierrors.Is(err, database.ErrEpochPruned) {
		return ierrors.Errorf("expected epoch %d to be pruned when calling Retainer", epoch)
	}

	return nil
}

func (t *TestSuite) assertPrunedState(expected *types.Tuple[int, bool], prunedStateFunc func() (iotago.EpochIndex, bool), name string) error {
	lastPruned, hasPruned := prunedStateFunc()

	if iotago.EpochIndex(expected.A) != lastPruned {
		return ierrors.Errorf("%s: expected %d, got %d", name, expected.A, lastPruned)
	}

	if expected.B != hasPruned {
		return ierrors.Errorf("%s: expected %v, got %v", name, expected.B, hasPruned)
	}

	return nil
}
