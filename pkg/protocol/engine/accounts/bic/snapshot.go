package bic

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

func (b *BICManager) Import(reader io.ReadSeeker) error {
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

	err := b.importBICTree(reader, accountCount)
	if err != nil {
		return errors.Wrapf(err, "unable to import BIC tree")
	}

	err = b.importSlotDiffs(reader, slotDiffCount)
	if err != nil {
		return errors.Wrap(err, "unable to import slot diffs")
	}

	return nil
}

func (b *BICManager) importSlotDiffs(reader io.ReadSeeker, slotDiffCount uint64) error {
	for i := uint64(0); i < slotDiffCount; i++ {
		var slotIndex iotago.SlotIndex
		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return errors.Wrap(err, "unable to read slot index")
		}
		var accountsCount uint64
		if err := binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return errors.Wrap(err, "unable to read accounts count")
		}
		diffStore := b.slotDiffFunc(slotIndex)

		for j := uint64(0); j < accountsCount; j++ {
			bicDiffChange, accountID, destroyed, err := SlotDiffSnapshotReader(reader)
			if err != nil {
				return errors.Wrapf(err, "unable to read slot diff")
			}
			err = diffStore.Store(accountID, *bicDiffChange, destroyed)
			if err != nil {
				return errors.Wrapf(err, "unable to store slot diff")
			}
		}
	}
	return nil
}

func (b *BICManager) importBICTree(reader io.ReadSeeker, accountCount uint64) error {
	// populate the bic tree, bic tree should be empty at this point
	for i := uint64(0); i < accountCount; i++ {
		accountData, err := AccountDataFromSnapshotReader(b.api, reader)
		if err != nil {
			return errors.Wrapf(err, "unable to read account data")
		}
		b.bicTree.Set(accountData.ID(), accountData)
	}
	return nil
}

func (b *BICManager) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
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

	accountCount, err := b.exportTargetBIC(pWriter, targetIndex)
	if err != nil {
		return errors.Wrapf(err, "unable to export BIC for target index %d", targetIndex)
	}

	if err := pWriter.WriteValueAtBookmark("accounts count", accountCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}

	if slotDiffCount, err = b.SlotDiffsSnapshotWriter(pWriter, targetIndex); err != nil {
		return errors.Wrapf(err, "unable to export slot diffs for target index %d", targetIndex)
	}

	if err = pWriter.WriteValueAtBookmark("slot diff count", slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	return nil
}

// exportTargetBIC exports the BICTree at a certain target slot, returning the total amount of exported accounts
func (b *BICManager) exportTargetBIC(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (accountCount uint64, err error) {
	if err = b.bicTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountData) bool {
		_, err = b.rollbackAccountTo(targetIndex, accountID, accountData)
		if err != nil {
			return false
		}
		err = AccountDataSnapshotWriter(pWriter, accountData)
		if err != nil {
			return false
		}
		accountCount++
		return true
	}); err != nil {
		return 0, errors.Wrap(err, "error in exporting BIC tree")
	}

	// we might have entries that were destroyed, that are present in diff, but not in the tree from the latestCommittedIndex
	accountCount, err = b.includeDestroyedAccountsToTargetBIC(pWriter, targetIndex, accountCount)

	return accountCount, err
}

func (b *BICManager) includeDestroyedAccountsToTargetBIC(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex, accountCount uint64) (uint64, error) {
	destroyedAccounts := make(map[iotago.AccountID]*accounts.AccountData)
	for index := b.latestCommittedSlot; index >= targetIndex+1; index-- {
		diffStore := b.slotDiffFunc(index)
		diffStore.StreamDestroyed(func(accountID iotago.AccountID) bool {
			bicDiffChange, _, err := diffStore.Load(accountID)
			if err != nil {
				return false
			}
			// TODO make sure that when we store diff, there are no duplicates beftween removed and added pubKeys,
			//  if key is present in both maps it should be removed from both
			accountData := accounts.NewAccountData(b.api, accountID, accounts.NewCredits(-bicDiffChange.Change, index), bicDiffChange.PubKeysRemoved...)

			destroyedAccounts[accountID] = accountData
			accountCount++
			return true
		})
	}
	for accountID, accountData := range destroyedAccounts {
		_, err := b.rollbackAccountTo(targetIndex, accountID, accountData)
		if err != nil {
			return accountCount, errors.Wrapf(err, "unable to rollback account %s to target slot index %d", accountID.String(), targetIndex)
		}
		err = AccountDataSnapshotWriter(pWriter, accountData)
		if err != nil {
			return accountCount, errors.Wrapf(err, "unable to write account %s to snapshot", accountID.String())
		}
	}

	return accountCount, nil
}

func (b *BICManager) SlotDiffsSnapshotWriter(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (slotDiffCount uint64, err error) {
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

		if err = b.slotDiffFunc(index).Stream(func(accountID iotago.AccountID, bicDiffChange prunable.BicDiffChange, destroyed bool) bool {
			err = SlotDiffSnapshotWriter(pWriter, accountID, bicDiffChange, destroyed)
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

func AccountDataFromSnapshotReader(api iotago.API, reader io.ReadSeeker) (*accounts.AccountData, error) {
	accountID, err := accountIDFromSnapshotReader(reader)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read account ID")
	}
	var value int64
	if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
		return nil, errors.Wrap(err, "unable to read BIC balance value")
	}
	var updatedTime iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &updatedTime); err != nil {
		return nil, errors.Wrap(err, "unable to readupdated time for BIC balance")
	}
	credits := &accounts.Credits{
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
	accountData := accounts.NewAccountData(api, accountID, credits, pubKeys...)
	return accountData, nil
}

func SlotDiffSnapshotWriter(writer *utils.PositionedWriter, accountID iotago.AccountID, bicDiffChange prunable.BicDiffChange, destroyed bool) error {
	slotDiffBytes := slotDiffSnapshotBytes(accountID, bicDiffChange, destroyed)
	if err := writer.WriteBytes(slotDiffBytes); err != nil {
		return errors.Wrap(err, "unable to write slot diff bytes")
	}
	return nil
}

func slotDiffSnapshotBytes(accountID iotago.AccountID, bicDiffChange prunable.BicDiffChange, destroyed bool) []byte {
	m := marshalutil.New()
	m.WriteBytes(lo.PanicOnErr(accountID.Bytes()))
	m.WriteInt64(bicDiffChange.Change)
	m.WriteUint64(uint64(bicDiffChange.PreviousUpdatedTime))
	// Length of the added public keys slice.
	m.WriteUint64(uint64(len(bicDiffChange.PubKeysAdded)))
	for _, addedPubKey := range bicDiffChange.PubKeysAdded {
		m.WriteBytes(lo.PanicOnErr(addedPubKey.Bytes()))
	}
	// Length of the removed public keys slice.
	m.WriteUint64(uint64(len(bicDiffChange.PubKeysRemoved)))
	for _, removedPubKey := range bicDiffChange.PubKeysRemoved {
		m.WriteBytes(lo.PanicOnErr(removedPubKey.Bytes()))
	}
	m.WriteBool(destroyed)
	return m.Bytes()
}

func SlotDiffSnapshotReader(reader io.ReadSeeker) (*prunable.BicDiffChange, iotago.AccountID, bool, error) {
	bicDiffChange := &prunable.BicDiffChange{
		PubKeysAdded:   make([]ed25519.PublicKey, 0),
		PubKeysRemoved: make([]ed25519.PublicKey, 0),
	}
	accountID, err := accountIDFromSnapshotReader(reader)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read account ID")
	}
	if err = binary.Read(reader, binary.LittleEndian, &bicDiffChange.Change); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read BIC balance value in the diff")
	}
	var updatedTime uint64
	if err = binary.Read(reader, binary.LittleEndian, &updatedTime); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read updated time in the diff")
	}
	bicDiffChange.PreviousUpdatedTime = iotago.SlotIndex(updatedTime)
	updatedKeys, err := pubKeysFromSnapshotReader(reader, bicDiffChange.PubKeysAdded)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	bicDiffChange.PubKeysAdded = updatedKeys
	updatedKeys, err = pubKeysFromSnapshotReader(reader, bicDiffChange.PubKeysRemoved)
	if err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrap(err, "unable to read added pubKeys in the diff")
	}
	bicDiffChange.PubKeysRemoved = updatedKeys
	var destroyed bool
	if err = binary.Read(reader, binary.LittleEndian, &destroyed); err != nil {
		return nil, iotago.AccountID{}, false, errors.Wrapf(err, "unable to read destroyed flag in the diff")
	}
	return bicDiffChange, accountID, destroyed, nil
}

func pubKeysFromSnapshotReader(reader io.ReadSeeker, pubKeysToUpdate []ed25519.PublicKey) ([]ed25519.PublicKey, error) {
	var pubKeysLength uint64
	if err := binary.Read(reader, binary.LittleEndian, &pubKeysLength); err != nil {
		return nil, errors.Wrapf(err, "unable to read added pubKeys length in the diff")
	}
	for k := uint64(0); k < pubKeysLength; k++ {
		pubKey, err := pubKeyFromSnapshotReader(reader)
		if err != nil {
			return nil, err
		}
		pubKeysToUpdate = append(pubKeysToUpdate, pubKey)
	}
	return pubKeysToUpdate, nil
}

func pubKeyFromSnapshotReader(reader io.ReadSeeker) (ed25519.PublicKey, error) {
	var pubKey ed25519.PublicKey
	if _, err := io.ReadFull(reader, pubKey[:]); err != nil {
		return ed25519.PublicKey{}, fmt.Errorf("unable to read public key: %w", err)
	}
	return pubKey, nil
}

func accountIDFromSnapshotWriter(writer *utils.PositionedWriter, accountID iotago.AccountID) error {
	accountIDBytes, err := accountID.Bytes()
	if err != nil {
		return err
	}
	if err = writer.WriteBytes(accountIDBytes); err != nil {
		return errors.Wrapf(err, "unable to write account id %s", accountID.String())
	}
	return nil
}

func accountIDFromSnapshotReader(reader io.ReadSeeker) (iotago.AccountID, error) {
	var accountID iotago.AccountID
	if _, err := io.ReadFull(reader, accountID[:]); err != nil {
		return iotago.AccountID{}, fmt.Errorf("unable to read LS output ID: %w", err)
	}
	return accountID, nil
}

func AccountDataSnapshotWriter(writer *utils.PositionedWriter, accountData *accounts.AccountData) error {
	accountBytes, err := accountData.SnapshotBytes()
	if err != nil {
		return errors.Wrap(err, "unable to get account data snapshot bytes")
	}
	if err = writer.WriteValue("account data", accountBytes); err != nil {
		return errors.Wrapf(err, "unable to write account data for account id %s", accountData.ID().String())
	}
	return nil
}
