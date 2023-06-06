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

func (b *Manager) Import(reader io.ReadSeeker) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	var accountCount uint64
	var slotDiffCount uint64
	// The amount of accounts contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &accountCount); err != nil {
		return errors.Wrap(err, "unable to read account count")
	}
	// The amount of slot diffs contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to read slot diffs count")
	}

	err := b.importAccountTree(reader, accountCount)
	if err != nil {
		return errors.Wrapf(err, "unable to import Account tree")
	}

	err = b.readSlotDiffs(reader, slotDiffCount)
	if err != nil {
		return errors.Wrap(err, "unable to import slot diffs")
	}

	return nil
}

func (b *Manager) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	var accountCount uint64
	var slotDiffCount uint64

	pWriter := utils.NewPositionedWriter(writer)

	if err := pWriter.WriteValue("accounts count", accountCount, true); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	if err := pWriter.WriteValue("slot diffs count", slotDiffCount, true); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	accountCount, err := b.exportAccountTree(pWriter, targetIndex)
	if err != nil {
		return errors.Wrapf(err, "unable to export Account for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("accounts count", accountCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}

	if slotDiffCount, err = b.writeSlotDiffs(pWriter, targetIndex); err != nil {
		return errors.Wrapf(err, "unable to export slot diffs for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("slot diff count", slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	return nil
}

func (b *Manager) importAccountTree(reader io.ReadSeeker, accountCount uint64) error {
	// populate the account tree, account tree should be empty at this point
	for i := uint64(0); i < accountCount; i++ {
		accountData, err := readAccountData(b.api, reader)
		if err != nil {
			return errors.Wrapf(err, "unable to read account data")
		}
		b.accountsTree.Set(accountData.ID(), accountData)
	}
	return nil
}

// exportAccountTree exports the AccountTree at a certain target slot, returning the total amount of exported accounts
func (b *Manager) exportAccountTree(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (accountCount uint64, err error) {
	if err = b.accountsTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountData) bool {
		_, err = b.rollbackAccountTo(accountData, targetIndex)
		if err != nil {
			return false
		}

		err = writeAccountData(pWriter, accountData)
		if err != nil {
			return false
		}

		accountCount++

		return true
	}); err != nil {
		return 0, errors.Wrap(err, "error in streaming Account tree")
	}

	// we might have entries that were destroyed, that are present in diffs, but not in the tree from the latestCommittedIndex we streamed above
	recreatedAccountsCount, err := b.recreateDestroyedAccounts(pWriter, targetIndex)

	return accountCount + recreatedAccountsCount, err
}

func (b *Manager) recreateDestroyedAccounts(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (recreatedAccountsCount uint64, err error) {
	destroyedAccounts := make(map[iotago.AccountID]*accounts.AccountData)
	for index := b.latestCommittedSlot; index >= targetIndex+1; index-- {
		diffStore := b.slotDiff(index)
		diffStore.StreamDestroyed(func(accountID iotago.AccountID) bool {
			accountDiffChange, _, err := diffStore.Load(accountID)
			if err != nil {
				return false
			}
			// TODO make sure that when we store diff, there are no duplicates beftween removed and added pubKeys,
			//  if key is present in both maps it should be removed from both
			// TODO: check if NewOutputID or PreviousOutputID
			accountData := accounts.NewAccountData(b.api, accountID, accounts.NewBlockIssuanceCredits(-accountDiffChange.Change, index), accountDiffChange.NewOutputID, accountDiffChange.PubKeysRemoved...)

			destroyedAccounts[accountID] = accountData
			recreatedAccountsCount++
			return true
		})
	}
	for accountID, accountData := range destroyedAccounts {
		_, err := b.rollbackAccountTo(accountData, targetIndex)
		if err != nil {
			return recreatedAccountsCount, errors.Wrapf(err, "unable to rollback account %s to target slot index %d", accountID.String(), targetIndex)
		}
		err = writeAccountData(pWriter, accountData)
		if err != nil {
			return recreatedAccountsCount, errors.Wrapf(err, "unable to write account %s to snapshot", accountID.String())
		}
	}

	return recreatedAccountsCount, nil
}

func (b *Manager) readSlotDiffs(reader io.ReadSeeker, slotDiffCount uint64) error {
	for i := uint64(0); i < slotDiffCount; i++ {
		var slotIndex iotago.SlotIndex
		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return errors.Wrap(err, "unable to read slot index")
		}
		var accountsCount uint64
		if err := binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return errors.Wrap(err, "unable to read accounts count")
		}
		diffStore := b.slotDiff(slotIndex)

		for j := uint64(0); j < accountsCount; j++ {
			accountDiff, accountID, destroyed, err := readSlotDiff(reader)
			if err != nil {
				return errors.Wrapf(err, "unable to read slot diff")
			}
			err = diffStore.Store(accountID, *accountDiff, destroyed)
			if err != nil {
				return errors.Wrapf(err, "unable to store slot diff")
			}
		}
	}
	return nil
}

func readSlotDiff(reader io.ReadSeeker) (*prunable.AccountDiff, iotago.AccountID, bool, error) {
	accountDiff := &prunable.AccountDiff{
		PubKeysAdded:   make([]ed25519.PublicKey, 0),
		PubKeysRemoved: make([]ed25519.PublicKey, 0),
	}
	accountID, err := readAccountID(reader)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read account ID")
	}
	if err = binary.Read(reader, binary.LittleEndian, &accountDiff.Change); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read Account balance value in the diff")
	}
	var updatedTime uint64
	if err = binary.Read(reader, binary.LittleEndian, &updatedTime); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read updated time in the diff")
	}
	accountDiff.PreviousUpdatedTime = iotago.SlotIndex(updatedTime)
	updatedKeys, err := readPubKeys(reader, accountDiff.PubKeysAdded)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	accountDiff.PubKeysAdded = updatedKeys
	updatedKeys, err = readPubKeys(reader, accountDiff.PubKeysRemoved)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	accountDiff.PubKeysRemoved = updatedKeys
	var destroyed bool
	if err = binary.Read(reader, binary.LittleEndian, &destroyed); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read destroyed flag in the diff")
	}
	return accountDiff, accountID, destroyed, nil
}

func (b *Manager) writeSlotDiffs(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (slotDiffCount uint64, err error) {
	for index := targetIndex - iotago.MaxCommitableSlotAge; index <= targetIndex; index++ {
		var accountsCount uint64
		// The index of the slot diff.
		if err = pWriter.WriteValue("index", index); err != nil {
			return slotDiffCount, err
		}
		// The amount of account entriess contained within this slot diff.
		if err = pWriter.WriteValue("diff count", accountsCount, true); err != nil {
			return slotDiffCount, err
		}

		if err = b.slotDiff(index).Stream(func(accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) bool {
			err = writeSlotDiff(pWriter, accountID, accountDiff, destroyed)
			if err != nil {
				panic(err)
			}
			accountsCount++
			return true
		}); err != nil {
			return slotDiffCount, errors.Wrapf(err, "unable to stream slot diff for index %d", index)
		}

		// The amount of slot diffs contained within this snapshot.
		if err = pWriter.WriteValueAtBookmark("diff count", accountsCount); err != nil {
			return slotDiffCount, err
		}

		slotDiffCount++
	}
	return slotDiffCount, nil
}

func writeSlotDiff(writer *utils.PositionedWriter, accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) error {
	slotDiffBytes := slotDiffBytes(accountID, accountDiff, destroyed)
	if err := writer.WriteBytes(slotDiffBytes); err != nil {
		return errors.Wrap(err, "unable to write slot diff bytes")
	}
	return nil
}

func readAccountData(api iotago.API, reader io.ReadSeeker) (*accounts.AccountData, error) {
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
		return nil, errors.Wrap(err, "unable to readupdated time for Account balance")
	}
	// TODO: import outputID for account
	var outputID iotago.OutputID
	credits := &accounts.BlockIssuanceCredits{
		Value:      value,
		UpdateTime: updatedTime,
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
	accountData := accounts.NewAccountData(api, accountID, credits, outputID, pubKeys...)
	return accountData, nil
}

func writeAccountData(writer *utils.PositionedWriter, accountData *accounts.AccountData) error {
	accountBytes, err := accountData.SnapshotBytes()
	if err != nil {
		return errors.Wrap(err, "unable to get account data snapshot bytes")
	}
	if err = writer.WriteValue("account data", accountBytes); err != nil {
		return errors.Wrapf(err, "unable to write account data for account id %s", accountData.ID().String())
	}
	return nil
}

func slotDiffBytes(accountID iotago.AccountID, accountDiff prunable.AccountDiff, destroyed bool) []byte {
	m := marshalutil.New()
	m.WriteBytes(lo.PanicOnErr(accountID.Bytes()))
	m.WriteInt64(accountDiff.Change)
	m.WriteUint64(uint64(accountDiff.PreviousUpdatedTime))
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

func writeAccountID(writer *utils.PositionedWriter, accountID iotago.AccountID) error {
	accountIDBytes, err := accountID.Bytes()
	if err != nil {
		return err
	}
	if err = writer.WriteBytes(accountIDBytes); err != nil {
		return errors.Wrapf(err, "unable to write account id %s", accountID.String())
	}
	return nil
}

func readAccountID(reader io.ReadSeeker) (iotago.AccountID, error) {
	var accountID iotago.AccountID
	if _, err := io.ReadFull(reader, accountID[:]); err != nil {
		return iotago.AccountID{}, fmt.Errorf("unable to read LS output ID: %w", err)
	}
	return accountID, nil
}
