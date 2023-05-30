package bic

import (
	"encoding/binary"
	"fmt"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/pkg/errors"
	"io"

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
		return fmt.Errorf("unable to read LS output count: %w", err)
	}
	// The amount of slot diffs contained within this snapshot.
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return fmt.Errorf("unable to read LS output count: %w", err)
	}

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

	// The amount of accounts contained within this snapshot.
	if err := pWriter.WriteValue("accounts count", accountCount, true); err != nil {
		return err
	}
	// The amount of slot diffs contained within this snapshot.
	if err := pWriter.WriteValue("slot diffs count", slotDiffCount, true); err != nil {
		return err
	}

	changesToBIC := b.BICDiffTo(targetIndex)
	b.bicTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountImpl) bool {
		if change, exists := changesToBIC[accountID]; exists {
			accountData.Credits().Value += change.Value
			accountData.Credits().UpdateTime = change.UpdateTime
		}
		accountIDBytes, err := accountID.Bytes()
		if err != nil {
			panic(err)
		}
		if err := pWriter.WriteBytes("account id", accountIDBytes); err != nil {
			panic(err)
		}

		if err := pWriter.WriteValue("account data", accountData.SnapshotBytes()); err != nil {
			panic(err)
		}

		accountCount++

		return true
	})

	for index := targetIndex - iotago.MaxCommitableSlotAge; index <= targetIndex; index++ {
		var diffCount uint64
		// The index of the slot diff.
		if err := pWriter.WriteValue("index", index); err != nil {
			return err
		}
		// The amount of slot diffs contained within this snapshot.
		if err := pWriter.WriteValue("diff count", diffCount, true); err != nil {
			return err
		}

		err := b.slotDiffFunc(index).Stream(func(accountID iotago.AccountID, change int64) bool {
			diffBytes := slotDiffSnapshotBytes(index, accountID, change)
			if err := pWriter.WriteBytes("diff", diffBytes); err != nil {
				panic(err)
			}
			diffCount++
			return true
		})
		if err != nil {
			return errors.Wrapf(err, "unable to stream slot diff for index %d", index)
		}
		// The amount of slot diffs contained within this snapshot.
		if err := pWriter.WriteValueAt("diff count", diffCount); err != nil {
			return err
		}

		slotDiffCount++
	}

	if err := pWriter.WriteValueAt("accounts count", accountCount); err != nil {
		return err
	}

	if err := pWriter.WriteValueAt("slot diff count", slotDiffCount); err != nil {
		return err
	}

	return nil
}

func (b *BICManager) accountDataFromSnapshotReader(reader io.ReadSeeker, id iotago.AccountID) (*accounts.AccountImpl, error) {
	var value int64
	if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
		return nil, errors.Wrapf(err, "unable to read LS output value")
	}
	var updatedTime iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &updatedTime); err != nil {
		return nil, errors.Wrapf(err, "unable to read LS output value")
	}
	credits := &accounts.Credits{
		Value:      value,
		UpdateTime: updatedTime,
	}
	var pubKeyCount int64
	if err := binary.Read(reader, binary.LittleEndian, &pubKeyCount); err != nil {
		return nil, errors.Wrapf(err, "unable to read LS pubKeyCount count")
	}
	pubKeys := make([]ed25519.PublicKey, pubKeyCount)
	for i := int64(0); i < pubKeyCount; i++ {
		var pubKey ed25519.PublicKey
		if _, err := io.ReadFull(reader, pubKey[:]); err != nil {
			return nil, fmt.Errorf("unable to read LS output ID: %w", err)
		}
		pubKeys[i] = pubKey
	}
	accountData := accounts.NewAccount(b.apiProviderFunc(), id, credits, pubKeys...)
	return accountData, nil
}

func slotDiffSnapshotBytes(index iotago.SlotIndex, accountID iotago.AccountID, value int64) []byte {
	m := marshalutil.New()
	m.WriteUint64(uint64(index))
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
