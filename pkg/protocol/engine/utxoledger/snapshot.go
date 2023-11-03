package utxoledger

import (
	"encoding/binary"
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
	outputID := iotago.OutputID{}
	if _, err := io.ReadFull(reader, outputID[:]); err != nil {
		return nil, ierrors.Errorf("unable to read LS output ID: %w", err)
	}

	blockID := iotago.BlockID{}
	if _, err := io.ReadFull(reader, blockID[:]); err != nil {
		return nil, ierrors.Errorf("unable to read LS block ID: %w", err)
	}

	var slotBooked iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &slotBooked); err != nil {
		return nil, ierrors.Errorf("unable to read LS output milestone index booked: %w", err)
	}

	var outputLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &outputLength); err != nil {
		return nil, ierrors.Errorf("unable to read LS output length: %w", err)
	}

	outputBytes := make([]byte, outputLength)
	if _, err := io.ReadFull(reader, outputBytes); err != nil {
		return nil, ierrors.Errorf("unable to read LS output bytes: %w", err)
	}

	var output iotago.TxEssenceOutput
	if _, err := apiProvider.APIForSlot(blockID.Slot()).Decode(outputBytes, &output, serix.WithValidation()); err != nil {
		return nil, ierrors.Errorf("invalid LS output address: %w", err)
	}

	var proofLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &proofLength); err != nil {
		return nil, ierrors.Errorf("unable to read LS output proof length: %w", err)
	}

	proofBytes := make([]byte, proofLength)
	if _, err := io.ReadFull(reader, proofBytes); err != nil {
		return nil, ierrors.Errorf("unable to read LS output proof bytes: %w", err)
	}

	proof, _, err := iotago.OutputIDProofFromBytes(apiProvider.APIForSlot(blockID.Slot()))(proofBytes)
	if err != nil {
		return nil, ierrors.Errorf("invalid LS output proof: %w", err)
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

	transactionIDSpent := iotago.TransactionID{}
	if _, err := io.ReadFull(reader, transactionIDSpent[:]); err != nil {
		return nil, ierrors.Errorf("unable to read LS transaction ID spent: %w", err)
	}

	return NewSpent(output, transactionIDSpent, indexSpent), nil
}

func ReadSlotDiffToSnapshotReader(reader io.ReadSeeker, apiProvider iotago.APIProvider) (*SlotDiff, error) {
	slotDiff := &SlotDiff{}

	var diffIndex iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &diffIndex); err != nil {
		return nil, ierrors.Errorf("unable to read slot diff index: %w", err)
	}
	slotDiff.Slot = diffIndex

	var createdCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &createdCount); err != nil {
		return nil, ierrors.Errorf("unable to read slot diff created count: %w", err)
	}

	slotDiff.Outputs = make(Outputs, createdCount)

	for i := uint64(0); i < createdCount; i++ {
		var err error
		slotDiff.Outputs[i], err = OutputFromSnapshotReader(reader, apiProvider)
		if err != nil {
			return nil, ierrors.Errorf("unable to read slot diff output: %w", err)
		}
	}

	var consumedCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &consumedCount); err != nil {
		return nil, ierrors.Errorf("unable to read slot diff consumed count: %w", err)
	}

	slotDiff.Spents = make(Spents, consumedCount)

	for i := uint64(0); i < consumedCount; i++ {
		var err error
		slotDiff.Spents[i], err = SpentFromSnapshotReader(reader, apiProvider, slotDiff.Slot)
		if err != nil {
			return nil, ierrors.Errorf("unable to read slot diff spent: %w", err)
		}
	}

	return slotDiff, nil
}

func WriteSlotDiffToSnapshotWriter(writer io.WriteSeeker, diff *SlotDiff) error {
	if err := stream.Write(writer, diff.Slot); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff index")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint64, func() (elementsCount int, err error) {
		for _, output := range diff.sortedOutputs() {
			if err := stream.WriteBytes(writer, output.SnapshotBytes()); err != nil {
				return 0, ierrors.Wrap(err, "unable to write slot diff created output")
			}
		}

		return len(diff.Outputs), nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write slot diff created collection")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint64, func() (elementsCount int, err error) {
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

	// TODO: change this to stream API and collections
	var snapshotLedgerIndex iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex); err != nil {
		return ierrors.Errorf("unable to read LS ledger index: %w", err)
	}

	if err := m.StoreLedgerIndexWithoutLocking(snapshotLedgerIndex); err != nil {
		return err
	}

	var outputCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &outputCount); err != nil {
		return ierrors.Errorf("unable to read LS output count: %w", err)
	}

	for i := uint64(0); i < outputCount; i++ {
		output, err := OutputFromSnapshotReader(reader, m.apiProvider)
		if err != nil {
			return ierrors.Errorf("at pos %d: %w", i, err)
		}

		if err := m.importUnspentOutputWithoutLocking(output); err != nil {
			return err
		}
	}

	var slotDiffCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return ierrors.Errorf("unable to read LS slot diff count: %w", err)
	}

	for i := uint64(0); i < slotDiffCount; i++ {
		slotDiff, err := ReadSlotDiffToSnapshotReader(reader, m.apiProvider)
		if err != nil {
			return err
		}

		if slotDiff.Slot != snapshotLedgerIndex-iotago.SlotIndex(i) {
			return ierrors.Errorf("invalid LS slot index. %d vs %d", slotDiff.Slot, snapshotLedgerIndex-iotago.SlotIndex(i))
		}

		if err := m.RollbackDiffWithoutLocking(slotDiff.Slot, slotDiff.Outputs, slotDiff.Spents); err != nil {
			return err
		}
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "unable to commit state tree")
	}

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
				return 0, ierrors.Wrapf(err, "error while retrieving output %s", outputID)
			}

			if err := stream.WriteBytes(writer, output.SnapshotBytes()); err != nil {
				return 0, ierrors.Wrapf(err, "unable to write output with ID %s", outputID)
			}

			outputCount++
		}

		return outputCount, nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to write unspent output collection")
	}

	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint64, func() (int, error) {
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

	return nil
}
