package bic

import (
	"fmt"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/pkg/errors"
	"io"

	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (b *BICManager) Import(reader io.ReadSeeker) error {
	// todo do we need storage for bic, what happens on the node startup?

	// TODO:
	// we will have one complete vector for the BIC for one slot in the snapshot file
	// latest commitment - MCA
	// then we also have the BIC diffs until latest committed slot index

	return nil
}

func (b *BICManager) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	var relativeCountersPosition int64
	var accountCount uint64
	var slotDiffCount uint64

	// The amount of accounts contained within this snapshot.
	if err := utils.WriteValueFunc(writer, "accounts count", accountCount, &relativeCountersPosition); err != nil {
		return err
	}
	// The amount of slot diffs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, "slot diffs count", slotDiffCount, &relativeCountersPosition); err != nil {
		return err
	}

	b.bicTree.Stream(func(accountID iotago.AccountID, accountData *accounts.AccountImpl) bool {
		accountIDBytes, err := accountID.Bytes()
		if err != nil {
			panic(err)
		}
		if err := utils.WriteBytesFunc(writer, "account id", accountIDBytes, &relativeCountersPosition); err != nil {
			panic(err)
		}

		if err := utils.WriteBytesFunc(writer, "account data", accountData.SnapshotBytes(), &relativeCountersPosition); err != nil {
			panic(err)
		}

		accountCount++

		return true
	})

	for index := b.bicIndex - iotago.MaxCommitableSlotAge; index <= b.bicIndex; index++ {
		var relativeDiffCountPosition int64
		var diffCount uint64
		// The index of the slot diff.
		if err := utils.WriteValueFunc(writer, "index", index, &relativeCountersPosition); err != nil {
			return err
		}
		// The amount of slot diffs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "diff count", diffCount, &relativeDiffCountPosition); err != nil {
			return err
		}

		err := b.slotDiffFunc(index).Stream(func(accountID iotago.AccountID, change int64) bool {
			diffBytes := slotDiffSnapshotBytes(index, accountID, change)
			if err := utils.WriteBytesFunc(writer, "diff", diffBytes, &relativeCountersPosition, &relativeDiffCountPosition); err != nil {
				panic(err)
			}
			diffCount++
			return true
		})
		if err != nil {
			return errors.Wrapf(err, "unable to stream slot diff for index %d", index)
		}
		// seek back to the last write position
		if _, err := writer.Seek(-relativeDiffCountPosition, io.SeekCurrent); err != nil {
			return fmt.Errorf("unable to seek to LS last written position: %w", err)
		}
		// The amount of slot diffs contained within this snapshot.
		if err := utils.WriteValueFunc(writer, "diff count", diffCount, &relativeCountersPosition); err != nil {
			return err
		}

		// seek forward to the write position on the end of the loop
		if _, err := writer.Seek(relativeDiffCountPosition-8, io.SeekCurrent); err != nil {
			return fmt.Errorf("unable to seek to LS last written position: %w", err)
		}
		slotDiffCount++
	}

	// seek back to the file position of the counters
	if _, err := writer.Seek(-relativeCountersPosition, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to LS counter placeholders: %w", err)
	}

	return nil
}

func slotDiffSnapshotBytes(index iotago.SlotIndex, accountID iotago.AccountID, value int64) []byte {
	m := marshalutil.New()
	m.WriteUint64(uint64(index))
	m.WriteBytes(lo.PanicOnErr(accountID.Bytes()))
	m.WriteInt64(value)
	return m.Bytes()
}
