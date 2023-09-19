package accountsledger

import (
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var accountCount uint64
	var slotDiffCount uint64

	// The number of accounts contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &accountCount); err != nil {
		return ierrors.Wrap(err, "unable to read account count")
	}

	// The number of slot diffs contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return ierrors.Wrap(err, "unable to read slot diffs count")
	}

	if err := m.importAccountTree(reader, accountCount); err != nil {
		return ierrors.Wrap(err, "unable to import account tree")
	}

	if err := m.readSlotDiffs(reader, slotDiffCount); err != nil {
		return ierrors.Wrap(err, "unable to import slot diffs")
	}

	return nil
}

func (m *Manager) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var accountCount uint64
	var slotDiffsCount uint64

	pWriter := utils.NewPositionedWriter(writer)

	if err := pWriter.WriteValue("accounts count", accountCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write accounts count")
	}

	if err := pWriter.WriteValue("slot diffs count", slotDiffsCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write slot diffs count")
	}

	accountCount, err := m.exportAccountTree(pWriter, targetIndex)
	if err != nil {
		return ierrors.Wrapf(err, "unable to export account for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("accounts count", accountCount); err != nil {
		return ierrors.Wrap(err, "unable to write accounts count")
	}

	if slotDiffsCount, err = m.writeSlotDiffs(pWriter, targetIndex); err != nil {
		return ierrors.Wrapf(err, "unable to export slot diffs for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("slot diffs count", slotDiffsCount); err != nil {
		return ierrors.Wrap(err, "unable to write slot diffs count")
	}

	return nil
}

func (m *Manager) importAccountTree(reader io.ReadSeeker, accountCount uint64) error {
	// populate the account tree, account tree should be empty at this point
	for i := uint64(0); i < accountCount; i++ {
		accountData := &accounts.AccountData{}
		if err := accountData.FromReader(reader); err != nil {
			return ierrors.Wrap(err, "unable to read account data")
		}

		if err := m.accountsTree.Set(accountData.ID, accountData); err != nil {
			return ierrors.Wrapf(err, "unable to set account %s", accountData.ID)
		}
	}

	return nil
}

// exportAccountTree exports the AccountTree at a certain target slot, returning the total amount of exported accounts.
func (m *Manager) exportAccountTree(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (accountCount uint64, err error) {
	if err = m.accountsTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountData) error {
		if _, err = m.rollbackAccountTo(accountData, targetIndex); err != nil {
			return ierrors.Wrapf(err, "unable to rollback account %s", accountID)
		}

		if err = writeAccountData(pWriter, accountData); err != nil {
			return ierrors.Wrapf(err, "unable to write data for account %s", accountID)
		}

		accountCount++

		return nil
	}); err != nil {
		return 0, ierrors.Wrap(err, "error in streaming account tree")
	}

	// we might have entries that were destroyed, that are present in diffs but not in the tree from the latestCommittedIndex we streamed above
	recreatedAccountsCount, err := m.recreateDestroyedAccounts(pWriter, targetIndex)

	return accountCount + recreatedAccountsCount, err
}

func (m *Manager) recreateDestroyedAccounts(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (recreatedAccountsCount uint64, err error) {
	destroyedAccounts := make(map[iotago.AccountID]*accounts.AccountData)

	for index := m.latestCommittedSlot; index > targetIndex; index-- {
		// it should be impossible that `m.slotDiff(index)` returns an error, because it is impossible to export a pruned slot
		err = lo.PanicOnErr(m.slotDiff(index)).StreamDestroyed(func(accountID iotago.AccountID) bool {
			// actual data will be filled in by rollbackAccountTo
			accountData := accounts.NewAccountData(accountID)

			destroyedAccounts[accountID] = accountData
			recreatedAccountsCount++

			return true
		})
		if err != nil {
			return 0, err
		}
	}

	for accountID, accountData := range destroyedAccounts {
		if wasDestroyed, err := m.rollbackAccountTo(accountData, targetIndex); err != nil {
			return 0, ierrors.Wrapf(err, "unable to rollback account %s to target slot index %d", accountID, targetIndex)
		} else if !wasDestroyed {
			return 0, ierrors.Errorf("account %s was not destroyed", accountID)
		}

		if err = writeAccountData(pWriter, accountData); err != nil {
			return 0, ierrors.Wrapf(err, "unable to write account %s to snapshot", accountID)
		}
	}

	return recreatedAccountsCount, nil
}

func writeAccountData(writer *utils.PositionedWriter, accountData *accounts.AccountData) error {
	accountBytes, err := accountData.Bytes()
	if err != nil {
		return ierrors.Wrapf(err, "unable to get account data snapshot bytes for accountID %s", accountData.ID)
	}

	if err = writer.WriteBytes(accountBytes); err != nil {
		return ierrors.Wrapf(err, "unable to write account data for accountID %s", accountData.ID)
	}

	return nil
}

func (m *Manager) readSlotDiffs(reader io.ReadSeeker, slotDiffCount uint64) error {
	for i := uint64(0); i < slotDiffCount; i++ {
		var slotIndex iotago.SlotIndex
		var accountsInDiffCount uint64

		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return ierrors.Wrap(err, "unable to read slot index")
		}

		if err := binary.Read(reader, binary.LittleEndian, &accountsInDiffCount); err != nil {
			return ierrors.Wrap(err, "unable to read accounts in diff count")
		}
		if accountsInDiffCount == 0 {
			continue
		}

		diffStore, err := m.slotDiff(slotIndex)
		if err != nil {
			return ierrors.Errorf("unable to import account slot diffs for slot %d", slotIndex)
		}

		for j := uint64(0); j < accountsInDiffCount; j++ {
			var accountID iotago.AccountID
			if _, err := io.ReadFull(reader, accountID[:]); err != nil {
				return ierrors.Wrapf(err, "unable to read accountID for index %d", j)
			}

			var destroyed bool
			if err := binary.Read(reader, binary.LittleEndian, &destroyed); err != nil {
				return ierrors.Wrapf(err, "unable to read destroyed flag for accountID %s", accountID)
			}

			accountDiff := model.NewAccountDiff()
			if !destroyed {
				if err := accountDiff.FromReader(reader); err != nil {
					return ierrors.Wrapf(err, "unable to read account diff for accountID %s", accountID)
				}
			}

			if err := diffStore.Store(accountID, accountDiff, destroyed); err != nil {
				return ierrors.Wrapf(err, "unable to store slot diff for accountID %s", accountID)
			}
		}
	}

	return nil
}

func (m *Manager) writeSlotDiffs(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (slotDiffsCount uint64, err error) {
	// write slot diffs until being able to reach targetIndex, where the exported tree is at
	slotIndex := iotago.SlotIndex(1)
	maxCommittableAge := m.apiProvider.APIForSlot(targetIndex).ProtocolParameters().MaxCommittableAge()

	if targetIndex > maxCommittableAge {
		slotIndex = targetIndex - maxCommittableAge
	}

	for ; slotIndex <= targetIndex; slotIndex++ {
		var accountsInDiffCount uint64

		// The index of the slot diffs.
		if err = pWriter.WriteValue("slot index", slotIndex); err != nil {
			return 0, err
		}

		// The number of account entries within this slot diff.
		if err = pWriter.WriteValue("inDiff accounts count", accountsInDiffCount, true); err != nil {
			return 0, err
		}

		slotDiffsCount++

		var innerErr error
		slotDiffs, err := m.slotDiff(slotIndex)
		if err != nil {
			// if slotIndex is already pruned, then don't write anything
			continue
		}

		if err = slotDiffs.Stream(func(accountID iotago.AccountID, accountDiff *model.AccountDiff, destroyed bool) bool {
			if err = pWriter.WriteBytes(lo.PanicOnErr(accountID.Bytes())); err != nil {
				innerErr = ierrors.Wrapf(err, "unable to write accountID for account %s", accountID)
			}

			if err = pWriter.WriteValue("destroyed flag", destroyed); err != nil {
				innerErr = ierrors.Wrapf(err, "unable to write destroyed flag for account %s", accountID)
			}

			if !destroyed {
				if err = pWriter.WriteBytes(lo.PanicOnErr(accountDiff.Bytes())); err != nil {
					innerErr = ierrors.Wrapf(err, "unable to write account diff for account %s", accountID)
				}
			}

			accountsInDiffCount++

			return true
		}); err != nil {
			return 0, ierrors.Wrapf(err, "unable to stream slot diff for index %d", slotIndex)
		}

		if innerErr != nil {
			return 0, ierrors.Wrapf(innerErr, "unable to write slot diff for index %d", slotIndex)
		}

		// The number of diffs contained within this slot.
		if err = pWriter.WriteValueAtBookmark("inDiff accounts count", accountsInDiffCount); err != nil {
			return 0, err
		}
	}

	return slotDiffsCount, nil
}
