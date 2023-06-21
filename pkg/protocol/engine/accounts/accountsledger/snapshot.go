package accountsledger

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
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
		return errors.Wrap(err, "unable to read account count")
	}

	// The number of slot diffs contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to read slot diffs count")
	}

	if err := m.importAccountTree(reader, accountCount); err != nil {
		return errors.Wrapf(err, "unable to import Account tree")
	}

	if err := m.readSlotDiffs(reader, slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to import slot diffs")
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
		return errors.Wrap(err, "unable to write accounts count")
	}
	if err := pWriter.WriteValue("slot diffs count", slotDiffsCount, true); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	accountCount, err := m.exportAccountTree(pWriter, targetIndex)
	if err != nil {
		return errors.Wrapf(err, "unable to export Account for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("accounts count", accountCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}

	if slotDiffsCount, err = m.writeSlotDiffs(pWriter, targetIndex); err != nil {
		return errors.Wrapf(err, "unable to export slot diffs for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("slot diffs count", slotDiffsCount); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	return nil
}

func (m *Manager) importAccountTree(reader io.ReadSeeker, accountCount uint64) error {
	// populate the account tree, account tree should be empty at this point
	for i := uint64(0); i < accountCount; i++ {
		accountData, err := readAccountData(m.api, reader)
		if err != nil {
			return errors.Wrapf(err, "unable to read account data")
		}
		m.accountsTree.Set(accountData.ID, accountData)
	}

	return nil
}

// exportAccountTree exports the AccountTree at a certain target slot, returning the total amount of exported accounts.
func (m *Manager) exportAccountTree(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (accountCount uint64, err error) {
	var innerErr error
	if err = m.accountsTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountData) bool {
		if _, err = m.rollbackAccountTo(accountData, targetIndex); err != nil {
			innerErr = errors.Wrapf(err, "unable to rollback account %s", accountID)
			return false
		}

		if err = writeAccountData(pWriter, accountData); err != nil {
			innerErr = errors.Wrapf(err, "unable to write data for account %s", accountID)
			return false
		}

		accountCount++

		return true
	}); err != nil {
		return 0, errors.Wrap(err, "error in streaming Account tree")
	} else if innerErr != nil {
		return 0, errors.Wrap(innerErr, "error in exporting account")
	}

	// we might have entries that were destroyed, that are present in diffs but not in the tree from the latestCommittedIndex we streamed above
	recreatedAccountsCount, err := m.recreateDestroyedAccounts(pWriter, targetIndex)

	return accountCount + recreatedAccountsCount, err
}

func (m *Manager) recreateDestroyedAccounts(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (recreatedAccountsCount uint64, err error) {
	destroyedAccounts := make(map[iotago.AccountID]*accounts.AccountData)

	for index := m.latestCommittedSlot; index > targetIndex; index-- {
		// no need to check if `m.slotDiff(index)` is nil, because it is impossible to export a pruned slot
		err = m.slotDiff(index).StreamDestroyed(func(accountID iotago.AccountID) bool {
			// actual data will be filled in by rollbackAccountTo
			accountData := accounts.NewAccountData(accountID, accounts.NewBlockIssuanceCredits(0, 0), iotago.OutputID{})

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
			return 0, errors.Wrapf(err, "unable to rollback account %s to target slot index %d", accountID.String(), targetIndex)
		} else if !wasDestroyed {
			return 0, errors.Errorf("account %s was not destroyed", accountID)
		}

		if err = writeAccountData(pWriter, accountData); err != nil {
			return 0, errors.Wrapf(err, "unable to write account %s to snapshot", accountID.String())
		}
	}

	return recreatedAccountsCount, nil
}

func (m *Manager) readSlotDiffs(reader io.ReadSeeker, slotDiffCount uint64) error {
	for i := uint64(0); i < slotDiffCount; i++ {
		var slotIndex iotago.SlotIndex
		var accountsInDiffCount uint64

		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return errors.Wrap(err, "unable to read slot index")
		}

		if err := binary.Read(reader, binary.LittleEndian, &accountsInDiffCount); err != nil {
			return errors.Wrap(err, "unable to read accounts count")
		}
		if accountsInDiffCount == 0 {
			continue
		}

		diffStore := m.slotDiff(slotIndex)
		if diffStore == nil {
			return errors.Errorf("unable to import account slot diffs for slot %d", slotIndex)
		}

		for j := uint64(0); j < accountsInDiffCount; j++ {
			accountID, accountDiff, destroyed, err := readSlotDiff(reader)
			if err != nil {
				return errors.Wrapf(err, "unable to read slot diff")
			}

			if err = diffStore.Store(accountID, *accountDiff, destroyed); err != nil {
				return errors.Wrapf(err, "unable to store slot diff")
			}
		}
	}

	return nil
}

func readSlotDiff(reader io.ReadSeeker) (accountID iotago.AccountID, accountDiff *prunable.AccountDiff, destroyed bool, err error) {
	accountDiff = prunable.NewAccountDiff()

	accountID, err = readAccountID(reader)
	if err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read account ID")
	}

	if err = binary.Read(reader, binary.LittleEndian, &accountDiff.Change); err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read Account balance value in the diff")
	}

	if err = binary.Read(reader, binary.LittleEndian, &accountDiff.PreviousUpdatedTime); err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read updated time in the diff")
	}

	if err = binary.Read(reader, binary.LittleEndian, &accountDiff.NewOutputID); err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read updated time in the diff")
	}

	if err = binary.Read(reader, binary.LittleEndian, &accountDiff.PreviousOutputID); err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read updated time in the diff")
	}

	updatedKeys, err := readPubKeys(reader, accountDiff.PubKeysAdded)
	if err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	accountDiff.PubKeysAdded = updatedKeys

	updatedKeys, err = readPubKeys(reader, accountDiff.PubKeysRemoved)
	if err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	accountDiff.PubKeysRemoved = updatedKeys

	if err = binary.Read(reader, binary.LittleEndian, &destroyed); err != nil {
		return iotago.AccountID{}, nil, false, errors.Wrapf(err, "unable to read destroyed flag in the diff")
	}

	return accountID, accountDiff, destroyed, nil
}

func (m *Manager) writeSlotDiffs(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (slotDiffsCount uint64, err error) {
	// write slot diffs until being able to reach targetIndex, where the exported tree is at
	slotIndex := iotago.SlotIndex(1)
	if targetIndex > m.maxCommittableAge {
		slotIndex = targetIndex - m.maxCommittableAge
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
		slotDiffs := m.slotDiff(slotIndex)
		if slotDiffs == nil {
			// if slotIndex is already pruned, then don't write anything
			continue
		}

		if err = slotDiffs.Stream(func(accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) bool {
			if err = writeSlotDiff(pWriter, accountID, accountDiff, destroyed); err != nil {
				innerErr = errors.Wrapf(err, "unable to write slot diff for account %s", accountID)
				return false
			}

			accountsInDiffCount++

			return true
		}); err != nil {
			return 0, errors.Wrapf(err, "unable to stream slot diff for index %d", slotIndex)
		}

		if innerErr != nil {
			return 0, errors.Wrapf(innerErr, "unable to write slot diff for index %d", slotIndex)
		}

		// The number of diffs contained within this slot.
		if err = pWriter.WriteValueAtBookmark("inDiff accounts count", accountsInDiffCount); err != nil {
			return 0, err
		}
	}

	return slotDiffsCount, nil
}

func writeSlotDiff(writer *utils.PositionedWriter, accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) error {
	if err := writer.WriteBytes(slotDiffBytes(accountID, accountDiff, destroyed)); err != nil {
		return errors.Wrap(err, "unable to write slot diff bytes")
	}

	return nil
}

func readAccountData(_ iotago.API, reader io.ReadSeeker) (*accounts.AccountData, error) {
	accountID, err := readAccountID(reader)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read account ID")
	}

	var value int64
	if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
		return nil, errors.Wrap(err, "unable to read Account balance value")
	}

	var updatedTime iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &updatedTime); err != nil {
		return nil, errors.Wrap(err, "unable to read updatedTime for Account balance")
	}

	var outputID iotago.OutputID
	if err := binary.Read(reader, binary.LittleEndian, &outputID); err != nil {
		return nil, errors.Wrap(err, "unable to read updatedTime for Account balance")
	}

	var pubKeyCount int64
	if err := binary.Read(reader, binary.LittleEndian, &pubKeyCount); err != nil {
		return nil, errors.Wrap(err, "unable to read pubKeyCount count")
	}

	pubKeys := make([]ed25519.PublicKey, pubKeyCount)
	for i := int64(0); i < pubKeyCount; i++ {
		var pubKey ed25519.PublicKey
		if _, err := io.ReadFull(reader, pubKey[:]); err != nil {
			return nil, errors.Wrap(err, "unable to read public key")
		}
		pubKeys[i] = pubKey
	}

	return accounts.NewAccountData(accountID, accounts.NewBlockIssuanceCredits(value, updatedTime), outputID, pubKeys...), nil
}

func writeAccountData(writer *utils.PositionedWriter, accountData *accounts.AccountData) error {
	accountBytes, err := accountData.Bytes()
	if err != nil {
		return errors.Wrap(err, "unable to get account data snapshot bytes")
	}

	if err = writer.WriteValue("account data", accountBytes); err != nil {
		return errors.Wrapf(err, "unable to write account data for account id %s", accountData.ID.String())
	}

	return nil
}

func slotDiffBytes(accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) []byte {
	m := marshalutil.New()
	m.WriteBytes(lo.PanicOnErr(accountID.Bytes()))
	m.WriteInt64(accountDiff.Change)
	m.WriteUint64(uint64(accountDiff.PreviousUpdatedTime))
	m.WriteBytes(lo.PanicOnErr(accountDiff.NewOutputID.Bytes()))
	m.WriteBytes(lo.PanicOnErr(accountDiff.PreviousOutputID.Bytes()))
	// Length of the added public keys slice.
	m.WriteUint64(uint64(len(accountDiff.PubKeysAdded)))
	for _, addedPubKey := range accountDiff.PubKeysAdded {
		m.WriteBytes(lo.PanicOnErr(addedPubKey.Bytes()))
	}
	// Length of the removed public keys slice.
	m.WriteUint64(uint64(len(accountDiff.PubKeysRemoved)))
	for _, removedPubKey := range accountDiff.PubKeysRemoved {
		m.WriteBytes(lo.PanicOnErr(removedPubKey.Bytes()))
	}
	m.WriteBool(destroyed)

	return m.Bytes()
}

func readPubKeys(reader io.ReadSeeker, pubKeysToUpdate []ed25519.PublicKey) ([]ed25519.PublicKey, error) {
	var pubKeysLength uint64
	if err := binary.Read(reader, binary.LittleEndian, &pubKeysLength); err != nil {
		return nil, errors.Wrapf(err, "unable to read added pubKeys length in the diff")
	}

	for k := uint64(0); k < pubKeysLength; k++ {
		pubKey, err := readPubKey(reader)
		if err != nil {
			return nil, err
		}
		pubKeysToUpdate = append(pubKeysToUpdate, pubKey)
	}

	return pubKeysToUpdate, nil
}

func readPubKey(reader io.ReadSeeker) (ed25519.PublicKey, error) {
	var pubKey ed25519.PublicKey
	if _, err := io.ReadFull(reader, pubKey[:]); err != nil {
		return ed25519.PublicKey{}, fmt.Errorf("unable to read public key: %w", err)
	}

	return pubKey, nil
}

func readAccountID(reader io.ReadSeeker) (iotago.AccountID, error) {
	var accountID iotago.AccountID
	if _, err := io.ReadFull(reader, accountID[:]); err != nil {
		return iotago.AccountID{}, fmt.Errorf("unable to read LS output ID: %w", err)
	}

	return accountID, nil
}
