package utxoledger_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/orcaman/writerseeker"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestOutput_SnapshotBytes(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	indexBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, indexBooked, iotaOutput, iotaOutputBytes)

	snapshotBytes := output.SnapshotBytes()

	require.Equal(t, outputID[:], snapshotBytes[:iotago.OutputIDLength], "outputID not equal")
	require.Equal(t, blockID[:], snapshotBytes[iotago.OutputIDLength:iotago.OutputIDLength+iotago.SlotIdentifierLength], "blockID not equal")
	require.Equal(t, iotago.SlotIndex(indexBooked), lo.Return1(iotago.SlotIndexFromBytes(snapshotBytes[iotago.OutputIDLength+iotago.SlotIdentifierLength:iotago.OutputIDLength+iotago.SlotIdentifierLength+iotago.SlotIndexLength])), "indexBooked not equal")
	require.Equal(t, uint32(len(iotaOutputBytes)), binary.LittleEndian.Uint32(snapshotBytes[iotago.OutputIDLength+iotago.SlotIdentifierLength+iotago.SlotIndexLength:iotago.OutputIDLength+iotago.SlotIdentifierLength+iotago.SlotIndexLength+4]), "output bytes length")
	require.Equal(t, iotaOutputBytes, snapshotBytes[iotago.OutputIDLength+iotago.SlotIdentifierLength+iotago.SlotIndexLength+4:], "output bytes not equal")
}

func TestOutputFromSnapshotReader(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	indexBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, indexBooked, iotaOutput, iotaOutputBytes)
	snapshotBytes := output.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readOutput, err := utxoledger.OutputFromSnapshotReader(buf, api.SingleVersionProvider(iotago_tpkg.TestAPI))
	require.NoError(t, err)

	require.Equal(t, output, readOutput)
}

func TestSpent_SnapshotBytes(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	indexBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, indexBooked, iotaOutput, iotaOutputBytes)
	outputSnapshotBytes := output.SnapshotBytes()

	transactionID := utils.RandTransactionID()
	indexSpent := utils.RandSlotIndex()
	spent := utxoledger.NewSpent(output, transactionID, indexSpent)

	snapshotBytes := spent.SnapshotBytes()

	require.Equal(t, outputSnapshotBytes, snapshotBytes[:len(outputSnapshotBytes)], "output bytes not equal")
	require.Equal(t, transactionID[:], snapshotBytes[len(outputSnapshotBytes):len(outputSnapshotBytes)+iotago.TransactionIDLength], "transactionID not equal")
}

func TestSpentFromSnapshotReader(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	indexBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, indexBooked, iotaOutput, iotaOutputBytes)

	transactionID := utils.RandTransactionID()
	indexSpent := utils.RandSlotIndex()
	spent := utxoledger.NewSpent(output, transactionID, indexSpent)

	snapshotBytes := spent.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readSpent, err := utxoledger.SpentFromSnapshotReader(buf, api.SingleVersionProvider(iotago_tpkg.TestAPI), indexSpent)
	require.NoError(t, err)

	require.Equal(t, spent, readSpent)
}

func TestReadSlotDiffToSnapshotReader(t *testing.T) {
	index := utils.RandSlotIndex()
	slotDiff := &utxoledger.SlotDiff{
		Index: index,
		Outputs: utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: utxoledger.Spents{
			tpkg.RandLedgerStateSpent(index),
			tpkg.RandLedgerStateSpent(index),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()
	readSlotDiff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
	require.NoError(t, err)

	require.Equal(t, slotDiff.Index, readSlotDiff.Index)
	tpkg.EqualOutputs(t, slotDiff.Outputs, readSlotDiff.Outputs)
	tpkg.EqualSpents(t, slotDiff.Spents, readSlotDiff.Spents)
}

func TestWriteSlotDiffToSnapshotWriter(t *testing.T) {
	index := utils.RandSlotIndex()
	slotDiff := &utxoledger.SlotDiff{
		Index: index,
		Outputs: utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: utxoledger.Spents{
			tpkg.RandLedgerStateSpent(index),
			tpkg.RandLedgerStateSpent(index),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()

	var readSlotIndex iotago.SlotIndex
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &readSlotIndex))
	require.Equal(t, iotago.SlotIndex(index), readSlotIndex)

	var createdCount uint64
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &createdCount))
	require.Equal(t, uint64(len(slotDiff.Outputs)), createdCount)

	var snapshotOutputs utxoledger.Outputs
	for i := 0; i < len(slotDiff.Outputs); i++ {
		readOutput, err := utxoledger.OutputFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, err)
		snapshotOutputs = append(snapshotOutputs, readOutput)
	}

	tpkg.EqualOutputs(t, slotDiff.Outputs, snapshotOutputs)

	var consumedCount uint64
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &consumedCount))
	require.Equal(t, uint64(len(slotDiff.Spents)), consumedCount)

	var snapshotSpents utxoledger.Spents
	for i := 0; i < len(slotDiff.Spents); i++ {
		readSpent, err := utxoledger.SpentFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI), iotago.SlotIndex(readSlotIndex))
		require.NoError(t, err)
		snapshotSpents = append(snapshotSpents, readSpent)
	}

	tpkg.EqualSpents(t, slotDiff.Spents, snapshotSpents)
}

func TestManager_Import(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := utxoledger.New(mapDB, api.SingleVersionProvider(iotago_tpkg.TestAPI))

	output1 := tpkg.RandLedgerStateOutput()

	require.NoError(t, manager.AddGenesisUnspentOutput(output1))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(0), ledgerIndex)

	mapDBAtIndex0 := mapdb.NewMapDB()
	// Copy the current manager state to the mapDBAtIndex0
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtIndex0))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, manager.ApplyDiff(1,
		utxoledger.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerIndex)

	mapDBAtIndex1 := mapdb.NewMapDB()
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtIndex1))

	require.NoError(t, manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerIndex)

	// Test exporting and importing at the current index 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		importedIndex2 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedIndex2.Import(reader))

		require.Equal(t, iotago.SlotIndex(2), lo.PanicOnErr(importedIndex2.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(manager.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex2.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at index 1
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.BytesReader()

		importedIndex1 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedIndex1.Import(reader))

		managerAtIndex1 := utxoledger.New(mapDBAtIndex1, api.SingleVersionProvider(iotago_tpkg.TestAPI))

		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(importedIndex1.ReadLedgerIndex()))
		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(managerAtIndex1.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(managerAtIndex1.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex1.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at index 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		importedIndex0 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedIndex0.Import(reader))

		managerAtIndex0 := utxoledger.New(mapDBAtIndex0, api.SingleVersionProvider(iotago_tpkg.TestAPI))

		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(importedIndex0.ReadLedgerIndex()))
		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(managerAtIndex0.ReadLedgerIndex()))
		require.Equal(t, lo.PanicOnErr(managerAtIndex0.LedgerStateSHA256Sum()), lo.PanicOnErr(importedIndex0.LedgerStateSHA256Sum()))
	}
}

func TestManager_Export(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := utxoledger.New(mapDB, api.SingleVersionProvider(iotago_tpkg.TestAPI))

	output1 := tpkg.RandLedgerStateOutput()

	require.NoError(t, manager.AddGenesisUnspentOutput(output1))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, manager.ApplyDiff(1,
		utxoledger.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1),
		}))

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerIndex)

	require.NoError(t, manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		}))

	ledgerIndex, err = manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerIndex)

	// Test exporting at the current index 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		var snapshotLedgerIndex iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(0), slotDiffCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
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

		var snapshotLedgerIndex iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(1), slotDiffCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		for i := uint64(0); i < slotDiffCount; i++ {
			diff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerIndex-iotago.SlotIndex(i), diff.Index)
		}
	}

	// Test exporting at index 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		var snapshotLedgerIndex iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerIndex))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerIndex)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var slotDiffCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint64(2), slotDiffCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		for i := uint64(0); i < slotDiffCount; i++ {
			diff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerIndex-iotago.SlotIndex(i), diff.Index)
		}
	}
}
