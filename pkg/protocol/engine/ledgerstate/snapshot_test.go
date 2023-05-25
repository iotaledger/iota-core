package ledgerstate_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestOutput_SnapshotBytes(t *testing.T) {
	outputID := tpkg.RandOutputID(2)
	blockID := tpkg.RandBlockID()
	indexBooked := tpkg.RandSlotIndex()
	slotCreated := tpkg.RandSlotIndex()
	iotaOutput := tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := tpkg.API().Encode(iotaOutput)
	require.NoError(t, err)

	output := ledgerstate.CreateOutput(tpkg.API(), outputID, blockID, indexBooked, slotCreated, iotaOutput, iotaOutputBytes)

	snapshotBytes := output.SnapshotBytes()

	require.Equal(t, outputID[:], snapshotBytes[:iotago.OutputIDLength], "outputID not equal")
	require.Equal(t, blockID[:], snapshotBytes[iotago.OutputIDLength:iotago.OutputIDLength+iotago.BlockIDLength], "blockID not equal")
	require.Equal(t, uint64(indexBooked), binary.LittleEndian.Uint64(snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength:iotago.OutputIDLength+iotago.BlockIDLength+8]), "slotIndexBooked not equal")
	require.Equal(t, uint64(slotCreated), int64(binary.LittleEndian.Uint64(snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength+8:iotago.OutputIDLength+iotago.BlockIDLength+8+8])), "slotIndexBooked not equal")
	require.Equal(t, uint32(len(iotaOutputBytes)), binary.LittleEndian.Uint32(snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength+8+8:iotago.OutputIDLength+iotago.BlockIDLength+8+8+4]), "output bytes length")
	require.Equal(t, iotaOutputBytes, snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength+8+8+4:], "output bytes not equal")
}

func TestOutputFromSnapshotReader(t *testing.T) {
	outputID := tpkg.RandOutputID(2)
	blockID := tpkg.RandBlockID()
	indexBooked := tpkg.RandSlotIndex()
	slotCreated := tpkg.RandSlotIndex()
	iotaOutput := tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := tpkg.API().Encode(iotaOutput)
	require.NoError(t, err)

	output := ledgerstate.CreateOutput(tpkg.API(), outputID, blockID, indexBooked, slotCreated, iotaOutput, iotaOutputBytes)
	snapshotBytes := output.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readOutput, err := ledgerstate.OutputFromSnapshotReader(buf, tpkg.API())
	require.NoError(t, err)

	require.Equal(t, output, readOutput)
}

func TestSpent_SnapshotBytes(t *testing.T) {
	outputID := tpkg.RandOutputID(2)
	blockID := tpkg.RandBlockID()
	indexBooked := tpkg.RandSlotIndex()
	slotCreated := tpkg.RandSlotIndex()
	iotaOutput := tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := tpkg.API().Encode(iotaOutput)
	require.NoError(t, err)

	output := ledgerstate.CreateOutput(tpkg.API(), outputID, blockID, indexBooked, slotCreated, iotaOutput, iotaOutputBytes)
	outputSnapshotBytes := output.SnapshotBytes()

	transactionID := tpkg.RandTransactionID()
	tsSpent := tpkg.RandTimestamp()
	indexSpent := tpkg.RandSlotIndex()
	spent := ledgerstate.NewSpent(output, transactionID, indexSpent)

	snapshotBytes := spent.SnapshotBytes()

	require.Equal(t, outputSnapshotBytes, snapshotBytes[:len(outputSnapshotBytes)], "output bytes not equal")
	require.Equal(t, transactionID[:], snapshotBytes[len(outputSnapshotBytes):len(outputSnapshotBytes)+iotago.TransactionIDLength], "transactionID not equal")
	require.Equal(t, tsSpent.UnixNano(), int64(binary.LittleEndian.Uint64(snapshotBytes[len(outputSnapshotBytes)+iotago.TransactionIDLength:])), "timestamp spent not equal")
}

func TestSpentFromSnapshotReader(t *testing.T) {
	outputID := tpkg.RandOutputID(2)
	blockID := tpkg.RandBlockID()
	indexBooked := tpkg.RandSlotIndex()
	slotCreated := tpkg.RandSlotIndex()
	iotaOutput := tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := tpkg.API().Encode(iotaOutput)
	require.NoError(t, err)

	output := ledgerstate.CreateOutput(tpkg.API(), outputID, blockID, indexBooked, slotCreated, iotaOutput, iotaOutputBytes)

	transactionID := tpkg.RandTransactionID()
	indexSpent := tpkg.RandSlotIndex()
	spent := ledgerstate.NewSpent(output, transactionID, indexSpent)

	snapshotBytes := spent.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readSpent, err := ledgerstate.SpentFromSnapshotReader(buf, tpkg.API(), indexSpent)
	require.NoError(t, err)

	require.Equal(t, spent, readSpent)
}

func TestReadSlotDiffToSnapshotReader(t *testing.T) {
	index := tpkg.RandSlotIndex()
	slotDiff := &ledgerstate.SlotDiff{
		Index: index,
		Outputs: ledgerstate.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: ledgerstate.Spents{
			tpkg.RandLedgerStateSpent(index, tpkg.RandTimestamp()),
			tpkg.RandLedgerStateSpent(index, tpkg.RandTimestamp()),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := ledgerstate.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()
	readSlotDiff, err := ledgerstate.ReadSlotDiffToSnapshotReader(reader, tpkg.API())
	require.NoError(t, err)

	require.Equal(t, slotDiff.Index, readSlotDiff.Index)
	tpkg.EqualOutputs(t, slotDiff.Outputs, readSlotDiff.Outputs)
	tpkg.EqualSpents(t, slotDiff.Spents, readSlotDiff.Spents)
}

func TestWriteSlotDiffToSnapshotWriter(t *testing.T) {
	index := tpkg.RandSlotIndex()
	slotDiff := &ledgerstate.SlotDiff{
		Index: index,
		Outputs: ledgerstate.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: ledgerstate.Spents{
			tpkg.RandLedgerStateSpent(index, tpkg.RandTimestamp()),
			tpkg.RandLedgerStateSpent(index, tpkg.RandTimestamp()),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := ledgerstate.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()

	var readSlotIndex uint64
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &readSlotIndex))
	require.Equal(t, uint64(index), readSlotIndex)

	var createdCount uint64
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &createdCount))
	require.Equal(t, uint64(len(slotDiff.Outputs)), createdCount)

	var snapshotOutputs ledgerstate.Outputs
	for i := 0; i < len(slotDiff.Outputs); i++ {
		readOutput, err := ledgerstate.OutputFromSnapshotReader(reader, tpkg.API())
		require.NoError(t, err)
		snapshotOutputs = append(snapshotOutputs, readOutput)
	}

	tpkg.EqualOutputs(t, slotDiff.Outputs, snapshotOutputs)

	var consumedCount uint64
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &consumedCount))
	require.Equal(t, uint64(len(slotDiff.Spents)), consumedCount)

	var snapshotSpents ledgerstate.Spents
	for i := 0; i < len(slotDiff.Spents); i++ {
		readSpent, err := ledgerstate.SpentFromSnapshotReader(reader, tpkg.API(), iotago.SlotIndex(readSlotIndex))
		require.NoError(t, err)
		snapshotSpents = append(snapshotSpents, readSpent)
	}

	tpkg.EqualSpents(t, slotDiff.Spents, snapshotSpents)
}

func TestManager_Import(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := ledgerstate.New(mapDB, tpkg.API)

	output1 := tpkg.RandLedgerStateOutput()

	require.NoError(t, manager.AddUnspentOutput(output1))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(0), ledgerIndex)

	mapDBAtIndex0 := mapdb.NewMapDB()
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtIndex0))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, manager.ApplyDiff(1,
		ledgerstate.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, ledgerstate.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1, tpkg.RandTimestamp()),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerIndex)

	mapDBAtIndex1 := mapdb.NewMapDB()
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtIndex1))

	require.NoError(t, manager.ApplyDiff(2,
		ledgerstate.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, ledgerstate.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2, tpkg.RandTimestamp()),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerIndex)

	// Test exporting and importing at the current index 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		importedIndex2 := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)
		require.NoError(t, importedIndex2.Import(reader))

		require.Equal(t, iotago.SlotIndex(2), lo.PanicOnErr(importedIndex2.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(manager.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex2.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at index 1
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.BytesReader()

		importedIndex1 := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)
		require.NoError(t, importedIndex1.Import(reader))

		managerAtIndex1 := ledgerstate.New(mapDBAtIndex1, tpkg.API)

		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(importedIndex1.ReadLedgerIndex()))
		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(managerAtIndex1.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(managerAtIndex1.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex1.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at index 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		importedIndex0 := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)
		require.NoError(t, importedIndex0.Import(reader))

		managerAtIndex0 := ledgerstate.New(mapDBAtIndex0, tpkg.API)

		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(importedIndex0.ReadLedgerIndex()))
		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(managerAtIndex0.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(managerAtIndex0.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex0.LedgerStateSHA256Sum()))
	}
}

func TestManager_Export(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := ledgerstate.New(mapDB, tpkg.API)

	output1 := tpkg.RandLedgerStateOutput()

	require.NoError(t, manager.AddUnspentOutput(output1))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddUnspentOutput(tpkg.RandLedgerStateOutput()))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, manager.ApplyDiff(1,
		ledgerstate.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, ledgerstate.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1, tpkg.RandTimestamp()),
		}))

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerIndex)

	require.NoError(t, manager.ApplyDiff(2,
		ledgerstate.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, ledgerstate.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2, tpkg.RandTimestamp()),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerIndex)

	// Test exporting at the current index 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		var snapshotLedgerIndex uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, uint64(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(0), slotDiffCount)

		var snapshotOutputs ledgerstate.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := ledgerstate.OutputFromSnapshotReader(reader, tpkg.API())
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		// Compare the snapshot outputs with our current ledger state
		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)
	}

	// Test exporting at index 1
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.BytesReader()

		var snapshotLedgerIndex uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, uint64(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(1), slotDiffCount)

		var snapshotOutputs ledgerstate.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := ledgerstate.OutputFromSnapshotReader(reader, tpkg.API())
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		for i := uint64(0); i < slotDiffCount; i++ {
			diff, err := ledgerstate.ReadSlotDiffToSnapshotReader(reader, tpkg.API())
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerIndex-i, uint64(diff.Index))
		}
	}

	// Test exporting at index 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		var snapshotLedgerIndex uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, uint64(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(2), slotDiffCount)

		var snapshotOutputs ledgerstate.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := ledgerstate.OutputFromSnapshotReader(reader, tpkg.API())
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		for i := uint64(0); i < slotDiffCount; i++ {
			diff, err := ledgerstate.ReadSlotDiffToSnapshotReader(reader, tpkg.API())
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerIndex-i, uint64(diff.Index))
		}
	}
}
