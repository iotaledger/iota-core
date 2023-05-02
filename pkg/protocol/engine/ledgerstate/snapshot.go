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
