package bic

import (
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/pkg/errors"
	"io"
)

func (b *BlockIssuanceCredits) Import(reader io.ReadSeeker) error {
	// todo do we need storage for bic, what happens on the node startup?

	// TODO:
	// we will have one complete vector for the BIC for one slot in the snapshot file
	// latest commitment - MCA
	// then we also have the BIC diffs until latest committed slot index

	return nil
}

func (b *BlockIssuanceCredits) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if err := utils.WriteValueFunc(writer, "balancesIndex", b.balancesIndex); err != nil {
		return err
	}
	if err := utils.WriteValueFunc(writer, "latestSlotDiffsIndex", b.latestSlotDiffsIndex); err != nil {
		return err
	}

	var relativeCountersPosition int64

	// TODO Write BIC to snapshot

	// write the slot diffs
	var slotDiffCount uint64

	// The amount of slot diffs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, "slot diffs count", slotDiffCount, &relativeCountersPosition); err != nil {
		return err
	}

	for diffIndex := b.balancesIndex + 1; diffIndex > b.latestSlotDiffsIndex; diffIndex-- {
		slotDiff, exists := b.slotDiffs.Get(diffIndex)
		if !exists {
			return errors.Errorf("could not export BIC diffs, slot diff for index %d not found", diffIndex)
		}
		written, err := WriteSlotDiffToSnapshotWriter(writer, slotDiff, b.apiProviderFunc())
		if err != nil {
			return err
		}

		relativeCountersPosition += written
		slotDiffCount++
	}

	return nil
}

func WriteSlotDiffToSnapshotWriter(writer io.WriteSeeker, diff *BICDiff, api iotago.API) (written int64, err error) {
	var totalBytesWritten int64

	if err := utils.WriteValueFunc(writer, "slot diff index", uint64(diff.index), &totalBytesWritten); err != nil {
		return 0, err
	}

	// TODO maybe its better to sort the map by outputID and then write the bytes one by one, in case diff is large
	allottmentBytes, err := api.Encode(diff.allotments)
	if err = utils.WriteBytesFunc(writer, "slot diff allottments", allottmentBytes, &totalBytesWritten); err != nil {
		return 0, err
	}

	burnBytes, err := api.Encode(diff.burns)
	if err = utils.WriteBytesFunc(writer, "slot diff burns", burnBytes, &totalBytesWritten); err != nil {
		return 0, err
	}
	return totalBytesWritten, nil
}
