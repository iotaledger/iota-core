package utxoledger

import (
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Helpers to serialize/deserialize into/from snapshots

func (o *Output) SnapshotBytes() []byte {
	m := marshalutil.New()
	m.WriteBytes(o.outputID[:])
	m.WriteBytes(o.blockID[:])
	m.WriteUint64(uint64(o.slotBooked))
	m.WriteUint64(uint64(o.slotCreated))
	m.WriteUint32(uint32(len(o.encodedOutput)))
	m.WriteBytes(o.encodedOutput)

	return m.Bytes()
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

	var indexBooked iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &indexBooked); err != nil {
		return nil, ierrors.Errorf("unable to read LS output milestone index booked: %w", err)
	}

	var indexCreated iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &indexCreated); err != nil {
		return nil, ierrors.Errorf("unable to read LS output index created: %w", err)
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
	if _, err := apiProvider.APIForSlot(blockID.Index()).Decode(outputBytes, &output, serix.WithValidation()); err != nil {
		return nil, ierrors.Errorf("invalid LS output address: %w", err)
	}

	return CreateOutput(apiProvider, outputID, blockID, indexBooked, indexCreated, output, outputBytes), nil
}

func (s *Spent) SnapshotBytes() []byte {
	m := marshalutil.New()
	m.WriteBytes(s.Output().SnapshotBytes())
	m.WriteBytes(s.transactionIDSpent[:])
	m.WriteBytes(s.slotIndexSpent.MustBytes())
	// we don't need to write indexSpent because this info is available in the milestoneDiff that consumes the output
	return m.Bytes()
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

	var timestampSpent int64
	if err := binary.Read(reader, binary.LittleEndian, &timestampSpent); err != nil {
		return nil, ierrors.Errorf("unable to read LS output milestone timestamp booked: %w", err)
	}

	return NewSpent(output, transactionIDSpent, indexSpent), nil
}

func ReadSlotDiffToSnapshotReader(reader io.ReadSeeker, apiProvider iotago.APIProvider) (*SlotDiff, error) {
	slotDiff := &SlotDiff{}

	var diffIndex uint64
	if err := binary.Read(reader, binary.LittleEndian, &diffIndex); err != nil {
		return nil, ierrors.Errorf("unable to read slot diff index: %w", err)
	}
	slotDiff.Index = iotago.SlotIndex(diffIndex)

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
		slotDiff.Spents[i], err = SpentFromSnapshotReader(reader, apiProvider, slotDiff.Index)
		if err != nil {
			return nil, ierrors.Errorf("unable to read slot diff spent: %w", err)
		}
	}

	return slotDiff, nil
}

func WriteSlotDiffToSnapshotWriter(writer io.WriteSeeker, diff *SlotDiff) (written int64, err error) {
	var totalBytesWritten int64

	if err := utils.WriteValueFunc(writer, uint64(diff.Index), &totalBytesWritten); err != nil {
		return 0, ierrors.Wrap(err, "unable to write slot diff index")
	}

	if err := utils.WriteValueFunc(writer, uint64(len(diff.Outputs)), &totalBytesWritten); err != nil {
		return 0, ierrors.Wrap(err, "unable to write slot diff created count")
	}

	for _, output := range diff.sortedOutputs() {
		if err := utils.WriteBytesFunc(writer, output.SnapshotBytes(), &totalBytesWritten); err != nil {
			return 0, ierrors.Wrap(err, "unable to write slot diff created output")
		}
	}

	if err := utils.WriteValueFunc(writer, uint64(len(diff.Spents)), &totalBytesWritten); err != nil {
		return 0, ierrors.Wrap(err, "unable to write slot diff consumed count")
	}

	for _, spent := range diff.sortedSpents() {
		if err := utils.WriteBytesFunc(writer, spent.SnapshotBytes(), &totalBytesWritten); err != nil {
			return 0, ierrors.Wrap(err, "unable to write slot diff created output")
		}
	}

	return totalBytesWritten, nil
}

// Import imports the ledger state from the given reader.
func (m *Manager) Import(reader io.ReadSeeker) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	var snapshotLedgerIndex uint64
	if err := binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex); err != nil {
		return ierrors.Errorf("unable to read LS ledger index: %w", err)
	}

	if err := m.StoreLedgerIndexWithoutLocking(iotago.SlotIndex(snapshotLedgerIndex)); err != nil {
		return err
	}

	var outputCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &outputCount); err != nil {
		return ierrors.Errorf("unable to read LS output count: %w", err)
	}

	var slotDiffCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return ierrors.Errorf("unable to read LS slot diff count: %w", err)
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

	for i := uint64(0); i < slotDiffCount; i++ {
		slotDiff, err := ReadSlotDiffToSnapshotReader(reader, m.apiProvider)
		if err != nil {
			return err
		}

		if slotDiff.Index != iotago.SlotIndex(snapshotLedgerIndex-i) {
			return ierrors.Errorf("invalid LS slot index. %d vs %d", slotDiff.Index, snapshotLedgerIndex-i)
		}

		if err := m.RollbackDiffWithoutLocking(slotDiff.Index, slotDiff.Outputs, slotDiff.Spents); err != nil {
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
	if err := utils.WriteValueFunc(writer, ledgerIndex); err != nil {
		return ierrors.Wrap(err, "unable to write ledger index")
	}

	var relativeCountersPosition int64

	var outputCount uint64
	var slotDiffCount uint64

	// Outputs Count
	// The amount of UTXOs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, outputCount, &relativeCountersPosition); err != nil {
		return ierrors.Wrap(err, "unable to write outputs count")
	}

	// Slot Diffs Count
	// The amount of slot diffs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, slotDiffCount, &relativeCountersPosition); err != nil {
		return ierrors.Wrap(err, "unable to write slot diffs count")
	}

	// Get all UTXOs and sort them by outputID
	outputIDs, err := m.UnspentOutputsIDs(ReadLockLedger(false))
	if err != nil {
		return ierrors.Wrap(err, "error while retrieving unspent outputIDs")
	}

	for _, outputID := range outputIDs.RemoveDupsAndSort() {
		output, err := m.ReadOutputByOutputIDWithoutLocking(outputID)
		if err != nil {
			return ierrors.Wrapf(err, "error while retrieving output %s", outputID)
		}

		if err := utils.WriteBytesFunc(writer, output.SnapshotBytes(), &relativeCountersPosition); err != nil {
			return ierrors.Wrap(err, "unable to write output ID")
		}

		outputCount++
	}

	for diffIndex := ledgerIndex; diffIndex > targetIndex; diffIndex-- {
		slotDiff, err := m.SlotDiffWithoutLocking(diffIndex)
		if err != nil {
			return ierrors.Wrapf(err, "error while retrieving slot diffs for slot %s", diffIndex)
		}

		written, err := WriteSlotDiffToSnapshotWriter(writer, slotDiff)
		if err != nil {
			return ierrors.Wrapf(err, "error while writing slot diffs for slot %s", diffIndex)
		}

		relativeCountersPosition += written
		slotDiffCount++
	}

	// seek back to the file position of the counters
	if _, err := writer.Seek(-relativeCountersPosition, io.SeekCurrent); err != nil {
		return ierrors.Errorf("unable to seek to LS counter placeholders: %w", err)
	}

	var countersSize int64

	// Outputs Count
	// The amount of UTXOs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, outputCount, &countersSize); err != nil {
		return ierrors.Wrap(err, "unable to write outputs count")
	}

	// Slot Diffs Count
	// The amount of slot diffs contained within this snapshot.
	if err := utils.WriteValueFunc(writer, slotDiffCount, &countersSize); err != nil {
		return ierrors.Wrap(err, "unable to write slot diffs count")
	}

	// seek back to the last write position
	if _, err := writer.Seek(relativeCountersPosition-countersSize, io.SeekCurrent); err != nil {
		return ierrors.Errorf("unable to seek to LS last written position: %w", err)
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

		if err := m.RollbackDiffWithoutLocking(slotDiff.Index, slotDiff.Outputs, slotDiff.Spents); err != nil {
			return err
		}
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "unable to commit state tree")
	}

	return nil
}
