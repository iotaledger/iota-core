package ledgerstate

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Helpers to serialize/deserialize into/from snapshots

func (o *Output) SnapshotBytes() []byte {
	m := marshalutil.New()
	m.WriteBytes(o.outputID[:])
	m.WriteBytes(o.blockID[:])
	m.WriteUint64(uint64(o.slotIndexBooked))
	m.WriteTime(o.timestampCreated)
	m.WriteUint32(uint32(len(o.encodedOutput)))
	m.WriteBytes(o.encodedOutput)

	return m.Bytes()
}

func OutputFromSnapshotReader(reader io.ReadSeeker, api iotago.API) (*Output, error) {
	outputID := iotago.OutputID{}
	if _, err := io.ReadFull(reader, outputID[:]); err != nil {
		return nil, fmt.Errorf("unable to read LS output ID: %w", err)
	}

	blockID := iotago.BlockID{}
	if _, err := io.ReadFull(reader, blockID[:]); err != nil {
		return nil, fmt.Errorf("unable to read LS block ID: %w", err)
	}

	var indexBooked iotago.SlotIndex
	if err := binary.Read(reader, binary.LittleEndian, &indexBooked); err != nil {
		return nil, fmt.Errorf("unable to read LS output milestone index booked: %w", err)
	}

	var timestampCreated int64
	if err := binary.Read(reader, binary.LittleEndian, &timestampCreated); err != nil {
		return nil, fmt.Errorf("unable to read LS output milestone timestamp booked: %w", err)
	}

	var outputLength uint32
	if err := binary.Read(reader, binary.LittleEndian, &outputLength); err != nil {
		return nil, fmt.Errorf("unable to read LS output length: %w", err)
	}

	outputBytes := make([]byte, outputLength)
	if _, err := io.ReadFull(reader, outputBytes); err != nil {
		return nil, fmt.Errorf("unable to read LS output bytes: %w", err)
	}

	var output iotago.TxEssenceOutput
	if _, err := api.Decode(outputBytes, &output, serix.WithValidation()); err != nil {
		return nil, fmt.Errorf("invalid LS output address: %w", err)
	}

	return CreateOutput(api, outputID, blockID, indexBooked, time.Unix(0, timestampCreated), output, outputBytes), nil
}

func (s *Spent) SnapshotBytes() []byte {
	m := marshalutil.New()
	m.WriteBytes(s.Output().SnapshotBytes())
	m.WriteBytes(s.transactionIDSpent[:])
	m.WriteTime(s.timestampSpent)
	// we don't need to write indexSpent because this info is available in the milestoneDiff that consumes the output
	return m.Bytes()
}

func SpentFromSnapshotReader(reader io.ReadSeeker, api iotago.API, indexSpent iotago.SlotIndex) (*Spent, error) {
	output, err := OutputFromSnapshotReader(reader, api)
	if err != nil {
		return nil, err
	}

	transactionIDSpent := iotago.TransactionID{}
	if _, err := io.ReadFull(reader, transactionIDSpent[:]); err != nil {
		return nil, fmt.Errorf("unable to read LS transaction ID spent: %w", err)
	}

	var timestampSpent int64
	if err := binary.Read(reader, binary.LittleEndian, &timestampSpent); err != nil {
		return nil, fmt.Errorf("unable to read LS output milestone timestamp booked: %w", err)
	}

	return NewSpent(output, transactionIDSpent, time.Unix(0, timestampSpent), indexSpent), nil
}

func ReadSlotDiffToSnapshotReader(reader io.ReadSeeker, api iotago.API) (*SlotDiff, error) {
	slotDiff := &SlotDiff{}

	var diffIndex uint64
	if err := binary.Read(reader, binary.LittleEndian, &diffIndex); err != nil {
		return nil, fmt.Errorf("unable to read slot diff index: %w", err)
	}
	slotDiff.Index = iotago.SlotIndex(diffIndex)

	var createdCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &createdCount); err != nil {
		return nil, fmt.Errorf("unable to read slot diff created count: %w", err)
	}

	slotDiff.Outputs = make(Outputs, createdCount)

	for i := uint64(0); i < createdCount; i++ {
		var err error
		slotDiff.Outputs[i], err = OutputFromSnapshotReader(reader, api)
		if err != nil {
			return nil, fmt.Errorf("unable to read slot diff output: %w", err)
		}
	}

	var consumedCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &consumedCount); err != nil {
		return nil, fmt.Errorf("unable to read slot diff consumed count: %w", err)
	}

	slotDiff.Spents = make(Spents, consumedCount)

	for i := uint64(0); i < consumedCount; i++ {
		var err error
		slotDiff.Spents[i], err = SpentFromSnapshotReader(reader, api, slotDiff.Index)
		if err != nil {
			return nil, fmt.Errorf("unable to read slot diff spent: %w", err)
		}
	}

	return slotDiff, nil
}

func WriteSlotDiffToSnapshotWriter(writer io.WriteSeeker, diff *SlotDiff) (written int64, err error) {
	var totalBytesWritten int64

	if err := writeValueFunc(writer, "slot diff index", uint64(diff.Index), &totalBytesWritten); err != nil {
		return 0, err
	}

	if err := writeValueFunc(writer, "slot diff created count", uint64(len(diff.Outputs)), &totalBytesWritten); err != nil {
		return 0, err
	}

	for _, output := range diff.sortedOutputs() {
		if err := writeBytesFunc(writer, "slot diff created output", output.SnapshotBytes(), &totalBytesWritten); err != nil {
			return 0, err
		}
	}

	if err := writeValueFunc(writer, "slot diff consumed count", uint64(len(diff.Spents)), &totalBytesWritten); err != nil {
		return 0, err
	}

	for _, spent := range diff.sortedSpents() {
		if err := writeBytesFunc(writer, "slot diff created output", spent.SnapshotBytes(), &totalBytesWritten); err != nil {
			return 0, err
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
		return fmt.Errorf("unable to read LS ledger index: %w", err)
	}

	var outputCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &outputCount); err != nil {
		return fmt.Errorf("unable to read LS output count: %w", err)
	}

	var slotDiffCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &slotDiffCount); err != nil {
		return fmt.Errorf("unable to read LS slot diff count: %w", err)
	}

	for i := uint64(0); i < outputCount; i++ {
		output, err := OutputFromSnapshotReader(reader, m.apiProviderFunc())
		if err != nil {
			return fmt.Errorf("at pos %d: %w", i, err)
		}

		if err := m.AddUnspentOutputWithoutLocking(output); err != nil {
			return err
		}
	}

	for i := uint64(0); i < slotDiffCount; i++ {
		slotDiff, err := ReadSlotDiffToSnapshotReader(reader, m.apiProviderFunc())
		if err != nil {
			return err
		}

		if slotDiff.Index != iotago.SlotIndex(snapshotLedgerIndex-1) {
			return fmt.Errorf("invalid LS slot index. %d vs %d", slotDiff.Index, snapshotLedgerIndex-1)
		}

		if err := m.RollbackConfirmation(slotDiff.Index, slotDiff.Outputs, slotDiff.Spents); err != nil {
			return err
		}

		snapshotLedgerIndex--
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
	if err := writeValueFunc(writer, "ledgerIndex", ledgerIndex); err != nil {
		return err
	}

	var relativeCountersPosition int64

	var outputCount uint64
	var slotDiffCount uint64

	// Outputs Count
	// The amount of UTXOs contained within this snapshot.
	if err := writeValueFunc(writer, "outputs count", outputCount, &relativeCountersPosition); err != nil {
		return err
	}

	// Slot Diffs Count
	// The amount of slot diffs contained within this snapshot.
	if err := writeValueFunc(writer, "slot diffs count", slotDiffCount, &relativeCountersPosition); err != nil {
		return err
	}

	// Get all UTXOs and sort them by outputID
	outputIDs, err := m.UnspentOutputsIDs(ReadLockLedger(false))
	if err != nil {
		return err
	}

	for _, outputID := range outputIDs.RemoveDupsAndSort() {
		output, err := m.ReadOutputByOutputID(outputID)
		if err != nil {
			return err
		}

		if err := writeBytesFunc(writer, "outputID", output.SnapshotBytes(), &relativeCountersPosition); err != nil {
			return err
		}

		outputCount++
	}

	for diffIndex := ledgerIndex; diffIndex >= targetIndex; diffIndex-- {
		slotDiff, err := m.SlotDiffWithoutLocking(diffIndex)
		if err != nil {
			return err
		}

		written, err := WriteSlotDiffToSnapshotWriter(writer, slotDiff)
		if err != nil {
			return err
		}

		relativeCountersPosition += written
		slotDiffCount++
	}

	// seek back to the file position of the counters
	if _, err := writer.Seek(-relativeCountersPosition, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to LS counter placeholders: %w", err)
	}

	// Outputs Count
	// The amount of UTXOs contained within this snapshot.
	if err := writeValueFunc(writer, "outputs count", outputCount); err != nil {
		return err
	}

	// Slot Diffs Count
	// The amount of slot diffs contained within this snapshot.
	if err := writeValueFunc(writer, "slot diffs count", slotDiffCount); err != nil {
		return err
	}

	// seek back to the last write position
	if _, err := writer.Seek(relativeCountersPosition, io.SeekCurrent); err != nil {
		return fmt.Errorf("unable to seek to LS last written position: %w", err)
	}

	return nil
}

func increaseOffsets(amount int64, offsets ...*int64) {
	for _, offset := range offsets {
		*offset += amount
	}
}

func writeValueFunc(writeSeeker io.WriteSeeker, variableName string, value any, offsetsToIncrease ...*int64) error {
	length := binary.Size(value)
	if length == -1 {
		return fmt.Errorf("unable to determine length of %s", variableName)
	}

	if err := binary.Write(writeSeeker, binary.LittleEndian, value); err != nil {
		return fmt.Errorf("unable to write LS %s: %w", variableName, err)
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}

func writeBytesFunc(writeSeeker io.WriteSeeker, variableName string, bytes []byte, offsetsToIncrease ...*int64) error {
	length, err := writeSeeker.Write(bytes)
	if err != nil {
		return fmt.Errorf("unable to write LS %s: %w", variableName, err)
	}

	increaseOffsets(int64(length), offsetsToIncrease...)

	return nil
}
