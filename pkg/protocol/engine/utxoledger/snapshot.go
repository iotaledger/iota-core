package utxoledger

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Helpers to serialize/deserialize into/from snapshots

func (o *Output) SnapshotBytes() []byte {
	return byteutils.ConcatBytes(o.outputID[:], o.KVStorableValue())
}

func OutputFromSnapshotReader(reader io.ReadSeeker, apiProvider iotago.APIProvider) (*Output, error) {
	outputID, err := stream.Read[iotago.OutputID](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS output ID")
	}

	blockID, err := stream.Read[iotago.BlockID](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS block ID")
	}

	slotBooked, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS output slot booked")
	}

	var outputBytes []byte
	output, err := stream.ReadObjectWithSize(reader, serializer.SeriLengthPrefixTypeAsUint32, func(bytes []byte) (iotago.TxEssenceOutput, int, error) {
		outputBytes = bytes

		var o iotago.TxEssenceOutput
		readBytes, err := apiProvider.APIForSlot(blockID.Slot()).Decode(bytes, &o, serix.WithValidation())
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "invalid LS output address")
		}

		return o, readBytes, nil
	})
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS output")
	}

	var proofBytes []byte
	proof, err := stream.ReadObjectWithSize(reader, serializer.SeriLengthPrefixTypeAsUint32, func(bytes []byte) (*iotago.OutputIDProof, int, error) {
		proofBytes = bytes

		proof, readBytes, err := iotago.OutputIDProofFromBytes(apiProvider.APIForSlot(blockID.Slot()))(proofBytes)
		if err != nil {
			return nil, 0, ierrors.Wrap(err, "invalid LS output proof")
		}

		return proof, readBytes, nil
	})
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS output proof")
	}

	return NewOutput(apiProvider, outputID, blockID, slotBooked, output, outputBytes, proof, proofBytes), nil
}

func (s *Spent) SnapshotBytes() []byte {
	// we don't need to write indexSpent because this info is available in the milestoneDiff that consumes the output

	return byteutils.ConcatBytes(s.Output().SnapshotBytes(), s.transactionIDSpent[:])
}

func SpentFromSnapshotReader(reader io.ReadSeeker, apiProvider iotago.APIProvider, indexSpent iotago.SlotIndex) (*Spent, error) {
	output, err := OutputFromSnapshotReader(reader, apiProvider)
	if err != nil {
		return nil, err
	}

	transactionIDSpent, err := stream.Read[iotago.TransactionID](reader)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to read LS transaction ID spent")
	}

	return NewSpent(output, transactionIDSpent, indexSpent), nil
}

func ReadSlotDiffToSnapshotReader(reader io.ReadSeeker, apiProvider iotago.APIProvider) (*SlotDiff, error) {
	var err error
	slotDiff := &SlotDiff{}

	if slotDiff.Slot, err = stream.Read[iotago.SlotIndex](reader); err != nil {
		return nil, ierrors.Wrap(err, "unable to read slot diff index")
	}

	createdCount, err := stream.PeekSize(reader, serializer.SeriLengthPrefixTypeAsUint32)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to peek slot diff created count")
	}
	slotDiff.Outputs = make(Outputs, createdCount)

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		slotDiff.Outputs[i], err = OutputFromSnapshotReader(reader, apiProvider)
		if err != nil {
			return ierrors.Wrap(err, "unable to read slot diff output")
		}

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "unable to read slot diff created collection")
	}

	consumedCount, err := stream.PeekSize(reader, serializer.SeriLengthPrefixTypeAsUint32)
	if err != nil {
		return nil, ierrors.Wrap(err, "unable to peek slot diff consumed count")
	}
	slotDiff.Spents = make(Spents, consumedCount)

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		slotDiff.Spents[i], err = SpentFromSnapshotReader(reader, apiProvider, slotDiff.Slot)
		if err != nil {
			return ierrors.Wrap(err, "unable to read slot diff spent")
		}

		return nil
	}); err != nil {
		return nil, ierrors.Wrap(err, "unable to read slot diff consumed collection")
	}

	return slotDiff, nil
}

func WriteSlotDiffToSnapshotWriter(writer io.WriteSeeker, diff *SlotDiff) error {
	if err := stream.Write(writer, diff.Slot); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff index")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for _, output := range diff.sortedOutputs() {
			if err := stream.WriteBytes(writer, output.SnapshotBytes()); err != nil {
				return 0, ierrors.Wrap(err, "unable to write slot diff created output")
			}
		}

		return len(diff.Outputs), nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff created collection")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for _, spent := range diff.sortedSpents() {
			if err := stream.WriteBytes(writer, spent.SnapshotBytes()); err != nil {
				return 0, ierrors.Wrap(err, "unable to write slot diff spent output")
			}
		}

		return len(diff.Spents), nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff spent collection")
	}

	return nil
}

// Import imports the ledger state from the given reader.
func (m *Manager) Import(reader io.ReadSeeker) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	snapshotLedgerIndex, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return ierrors.Wrap(err, "unable to read LS ledger index")
	}
	if err := m.StoreLedgerIndexWithoutLocking(snapshotLedgerIndex); err != nil {
		return err
	}

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint64, func(i int) error {
		output, err := OutputFromSnapshotReader(reader, m.apiProvider)
		if err != nil {
			return ierrors.Wrapf(err, "at pos %d", i)
		}

		if err := m.importUnspentOutputWithoutLocking(output); err != nil {
			return ierrors.Wrap(err, "unable to import LS output")
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to read LS output collection")
	}

	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		slotDiff, err := ReadSlotDiffToSnapshotReader(reader, m.apiProvider)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read LS slot diff at index %d", i)
		}

		if slotDiff.Slot != snapshotLedgerIndex-iotago.SlotIndex(i) {
			return ierrors.Errorf("invalid LS slot index, %d vs %d", slotDiff.Slot, snapshotLedgerIndex-iotago.SlotIndex(i))
		}

		if err := m.RollbackDiffWithoutLocking(slotDiff.Slot, slotDiff.Outputs, slotDiff.Spents); err != nil {
			return ierrors.Wrapf(err, "unable to rollback LS slot diff at index %d", i)
		}

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to read LS slot diff collection")
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "unable to commit state tree")
	}

	m.reInitStateTreeWithoutLocking()

	return nil
}

// Export exports the ledger state to the given writer.
func (m *Manager) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	m.ReadLockLedger()
	defer m.ReadUnlockLedger()

	ledgerIndex, err := m.ReadLedgerIndexWithoutLocking()
	if err != nil {
		return err
	}

	if err := stream.Write(writer, ledgerIndex); err != nil {
		return ierrors.Wrap(err, "unable to write ledger index")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint64, func() (int, error) {
		// Get all UTXOs and sort them by outputID
		outputIDs, err := m.UnspentOutputsIDs(ReadLockLedger(false))
		if err != nil {
			return 0, ierrors.Wrap(err, "error while retrieving unspent outputIDs")
		}

		var outputCount int
		for _, outputID := range outputIDs.RemoveDupsAndSort() {
			output, err := m.ReadOutputByOutputIDWithoutLocking(outputID)
			if err != nil {
				return 0, ierrors.Wrapf(err, "error while retrieving output with ID %s", outputID.ToHex())
			}

			if err := stream.WriteBytes(writer, output.SnapshotBytes()); err != nil {
				return 0, ierrors.Wrapf(err, "unable to write output with ID %s", outputID.ToHex())
			}

			outputCount++
		}

		return outputCount, nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write unspent output collection")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (int, error) {
		var slotDiffCount int
		for diffIndex := ledgerIndex; diffIndex > targetIndex; diffIndex-- {
			slotDiff, err := m.SlotDiffWithoutLocking(diffIndex)
			if err != nil {
				return 0, ierrors.Wrapf(err, "error while retrieving slot diffs for slot %s", diffIndex)
			}

			if WriteSlotDiffToSnapshotWriter(writer, slotDiff) != nil {
				return 0, ierrors.Wrapf(err, "error while writing slot diffs for slot %s", diffIndex)
			}

			slotDiffCount++
		}

		return slotDiffCount, nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff collection")
	}

	return nil
}

// Rollback rolls back ledger state to the given target slot.
func (m *Manager) Rollback(targetSlot iotago.SlotIndex) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	ledgerIndex, err := m.ReadLedgerIndexWithoutLocking()
	if err != nil {
		return err
	}

	for diffIndex := ledgerIndex; diffIndex > targetSlot; diffIndex-- {
		slotDiff, err := m.SlotDiffWithoutLocking(diffIndex)
		if err != nil {
			return err
		}

		if err := m.RollbackDiffWithoutLocking(slotDiff.Slot, slotDiff.Outputs, slotDiff.Spents); err != nil {
			return err
		}
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "unable to commit state tree")
	}

	m.reInitStateTreeWithoutLocking()

	return nil
}
