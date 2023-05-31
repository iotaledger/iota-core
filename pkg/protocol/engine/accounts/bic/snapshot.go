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
			accountID, err := accountIDFromSnapshotReader(reader)
			if err != nil {
				return errors.Wrapf(err, "unable to read account ID")
			}
			var value int64
			if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
				return errors.Wrapf(err, "unable to read BIC balance value in the diff")
			}
			err = diffStore.Store(accountID, value)
			if err != nil {
				return errors.Wrapf(err, "unable to store BIC balance value in the diff, slotIndex: %d, accountID: %s", slotIndex, accountID)
			}
		}
	}
	return nil
}

func (b *BICManager) importBICTree(reader io.ReadSeeker, accountCount uint64) error {
	// populate the bic tree, bic tree should be empty at this point
	for i := uint64(0); i < accountCount; i++ {
		accountID, err := accountIDFromSnapshotReader(reader)
		if err != nil {
			return errors.Wrapf(err, "unable to read account ID")
		}

		accountImpl, err := b.accountDataFromSnapshotReader(reader, accountID)
		if err != nil {
			return errors.Wrapf(err, "unable to read account data")
		}
		b.bicTree.Set(accountID, accountImpl)
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

	accountCount = b.exportTargetBIC(pWriter, targetIndex)

	var err error
	slotDiffCount, err = b.exportSlotDiffs(targetIndex, pWriter, slotDiffCount)

	if err = pWriter.WriteValueAtBookmark("accounts count", accountCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	if err = pWriter.WriteValueAtBookmark("slot diff count", slotDiffCount); err != nil {
		return errors.Wrap(err, "unable to write slot diffs count")
	}

	return nil
}

// exportTargetBIC exports the BICTree at a certain target slot, returning the total amount of exported accounts
func (b *BICManager) exportTargetBIC(pWriter *utils.PositionedWriter, targetIndex iotago.SlotIndex) (accountCount uint64) {
	changesToBIC := b.BICDiffTo(targetIndex)
	err := b.bicTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountData) bool {
		if change, exists := changesToBIC[accountID]; exists {
			accountData.Credits().Value += change.Value
			accountData.Credits().UpdateTime = change.UpdateTime
		}
		err := writeAccountID(pWriter, accountID)
		if err != nil {
			panic(err)
		}
		if err = pWriter.WriteValue("account data", accountData.SnapshotBytes()); err != nil {
			panic(err)
		}
		accountCount++

		return true
	})
	if err != nil {
		panic(err)
	}

	// we might have entries that were destroyed, that are present in diff, but not in the tree from the latestCommittedIndex
	accountCount = b.includeDestroyedAccountsToTargetBIC(pWriter, changesToBIC, accountCount)

	return accountCount
}

func (b *BICManager) exportSlotDiffs(targetIndex iotago.SlotIndex, pWriter *utils.PositionedWriter, slotDiffCount uint64) (uint64, error) {
	for index := targetIndex - iotago.MaxCommitableSlotAge; index <= targetIndex; index++ {
		var accountsCount uint64
		// The index of the slot diff.
		if err := pWriter.WriteValue("index", index); err != nil {
			return slotDiffCount, err
		}
		// The amount of account entriess contained within this slot diff.
		if err := pWriter.WriteValue("diff count", accountsCount, true); err != nil {
			return slotDiffCount, err
		}

		err := b.slotDiffFunc(index).Stream(func(accountID iotago.AccountID, change int64) bool {
			diffBytes := slotDiffSnapshotBytes(accountID, change)
			if err := pWriter.WriteBytes(diffBytes); err != nil {
				panic(errors.Wrap(err, "unable to write slot diff bytes"))
			}
			accountsCount++
			return true
		})
		if err != nil {
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

func (b *BICManager) includeDestroyedAccountsToTargetBIC(pWriter *utils.PositionedWriter, changesToBIC map[iotago.AccountID]*accounts.Credits, accountCount uint64) uint64 {
	for accountID := range changesToBIC {
		if exists := b.bicTree.Has(accountID); !exists {
			err := writeAccountID(pWriter, accountID)
			if err != nil {
				panic(err)
			}
			accountData := createNewAccountDataBasedOnChanges(accountID, changesToBIC[accountID], b.API())
			if err = pWriter.WriteValue("account data", accountData.SnapshotBytes()); err != nil {
				panic(err)
			}
			accountCount++
		}
	}
	return accountCount
}

func (b *BICManager) accountDataFromSnapshotReader(reader io.ReadSeeker, id iotago.AccountID) (*accounts.AccountData, error) {
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
	accountData := accounts.NewAccount(b.api, id, credits, pubKeys...)
	return accountData, nil
}

func slotDiffSnapshotBytes(accountID iotago.AccountID, value int64) []byte {
	m := marshalutil.New()
	m.WriteBytes(lo.PanicOnErr(accountID.Bytes()))
	m.WriteInt64(value)
	return m.Bytes()
}

func accountIDFromSnapshotReader(reader io.ReadSeeker) (iotago.AccountID, error) {
	var accountID iotago.AccountID
	if _, err := io.ReadFull(reader, accountID[:]); err != nil {
		return iotago.AccountID{}, fmt.Errorf("unable to read LS output ID: %w", err)
	}
	return accountID, nil
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

func createNewAccountDataBasedOnChanges(accountID iotago.AccountID, changes *accounts.Credits, api iotago.API) *accounts.AccountData {
	//  TODO store pubkeys for diffs pubKeys := make([]ed25519.PublicKey, 0)
	return accounts.NewAccount(api, accountID, changes)
}
