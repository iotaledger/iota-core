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
	slotBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes)

	snapshotBytes := output.SnapshotBytes()

	require.Equal(t, outputID[:], snapshotBytes[:iotago.OutputIDLength], "outputID not equal")
	require.Equal(t, blockID[:], snapshotBytes[iotago.OutputIDLength:iotago.OutputIDLength+iotago.BlockIDLength], "blockID not equal")
	require.Equal(t, slotBooked, lo.Return1(iotago.SlotIndexFromBytes(snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength:iotago.OutputIDLength+iotago.BlockIDLength+iotago.SlotIndexLength])), "slotBooked not equal")
	require.Equal(t, uint32(len(iotaOutputBytes)), binary.LittleEndian.Uint32(snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength+iotago.SlotIndexLength:iotago.OutputIDLength+iotago.BlockIDLength+iotago.SlotIndexLength+4]), "output bytes length")
	require.Equal(t, iotaOutputBytes, snapshotBytes[iotago.OutputIDLength+iotago.BlockIDLength+iotago.SlotIndexLength+4:], "output bytes not equal")
}

func TestOutputFromSnapshotReader(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	slotBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes)
	snapshotBytes := output.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readOutput, err := utxoledger.OutputFromSnapshotReader(buf, api.SingleVersionProvider(iotago_tpkg.TestAPI))
	require.NoError(t, err)

	require.Equal(t, output, readOutput)
}

func TestSpent_SnapshotBytes(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	slotBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes)
	outputSnapshotBytes := output.SnapshotBytes()

	transactionID := utils.RandTransactionID()
	slotSpent := utils.RandSlotIndex()
	spent := utxoledger.NewSpent(output, transactionID, slotSpent)

	snapshotBytes := spent.SnapshotBytes()

	require.Equal(t, outputSnapshotBytes, snapshotBytes[:len(outputSnapshotBytes)], "output bytes not equal")
	require.Equal(t, transactionID[:], snapshotBytes[len(outputSnapshotBytes):len(outputSnapshotBytes)+iotago.TransactionIDLength], "transactionID not equal")
}

func TestSpentFromSnapshotReader(t *testing.T) {
	outputID := utils.RandOutputID(2)
	blockID := utils.RandBlockID()
	slotBooked := utils.RandSlotIndex()
	iotaOutput := utils.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.TestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	output := utxoledger.CreateOutput(api.SingleVersionProvider(iotago_tpkg.TestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes)

	transactionID := utils.RandTransactionID()
	slotSpent := utils.RandSlotIndex()
	spent := utxoledger.NewSpent(output, transactionID, slotSpent)

	snapshotBytes := spent.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readSpent, err := utxoledger.SpentFromSnapshotReader(buf, api.SingleVersionProvider(iotago_tpkg.TestAPI), slotSpent)
	require.NoError(t, err)

	require.Equal(t, spent, readSpent)
}

func TestReadSlotDiffToSnapshotReader(t *testing.T) {
	slot := utils.RandSlotIndex()
	slotDiff := &utxoledger.SlotDiff{
		Slot: slot,
		Outputs: utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: utxoledger.Spents{
			tpkg.RandLedgerStateSpent(slot),
			tpkg.RandLedgerStateSpent(slot),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()
	readSlotDiff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI))
	require.NoError(t, err)

	require.Equal(t, slotDiff.Slot, readSlotDiff.Slot)
	tpkg.EqualOutputs(t, slotDiff.Outputs, readSlotDiff.Outputs)
	tpkg.EqualSpents(t, slotDiff.Spents, readSlotDiff.Spents)
}

func TestWriteSlotDiffToSnapshotWriter(t *testing.T) {
	slot := utils.RandSlotIndex()
	slotDiff := &utxoledger.SlotDiff{
		Slot: slot,
		Outputs: utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		},
		Spents: utxoledger.Spents{
			tpkg.RandLedgerStateSpent(slot),
			tpkg.RandLedgerStateSpent(slot),
		},
	}

	writer := &writerseeker.WriterSeeker{}
	written, err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	require.Equal(t, int64(writer.BytesReader().Len()), written)

	reader := writer.BytesReader()

	var readSlot iotago.SlotIndex
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &readSlot))
	require.Equal(t, slot, readSlot)

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
		readSpent, err := utxoledger.SpentFromSnapshotReader(reader, api.SingleVersionProvider(iotago_tpkg.TestAPI), readSlot)
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

	ledgerSlot, err := manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(0), ledgerSlot)

	mapDBAtSlot0 := mapdb.NewMapDB()
	// Copy the current manager state to the mapDBAtSlot0
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtSlot0))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, manager.ApplyDiff(1,
		utxoledger.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1),
		}))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerSlot)

	mapDBAtSlot1 := mapdb.NewMapDB()
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtSlot1))

	require.NoError(t, manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		}))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerSlot)

	// Test exporting and importing at the current slot 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		importedSlot2 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedSlot2.Import(reader))

		require.Equal(t, iotago.SlotIndex(2), lo.PanicOnErr(importedSlot2.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(manager.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot2.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at slot 1
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.BytesReader()

		importedSlot1 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedSlot1.Import(reader))

		managerAtSlot1 := utxoledger.New(mapDBAtSlot1, api.SingleVersionProvider(iotago_tpkg.TestAPI))

		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(importedSlot1.ReadLedgerSlot()))
		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(managerAtSlot1.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(managerAtSlot1.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot1.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at slot 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		importedSlot0 := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))
		require.NoError(t, importedSlot0.Import(reader))

		managerAtSlot0 := utxoledger.New(mapDBAtSlot0, api.SingleVersionProvider(iotago_tpkg.TestAPI))

		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(importedSlot0.ReadLedgerSlot()))
		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(managerAtSlot0.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(managerAtSlot0.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot0.LedgerStateSHA256Sum()))
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

	ledgerSlot, err := manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerSlot)

	require.NoError(t, manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		}))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerSlot)

	// Test exporting at the current slot 2
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.BytesReader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

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

	// Test exporting at slot 1
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.BytesReader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

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
			require.Equal(t, snapshotLedgerSlot-iotago.SlotIndex(i), diff.Slot)
		}
	}

	// Test exporting at slot 0
	{
		writer := &writerseeker.WriterSeeker{}
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.BytesReader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

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
			require.Equal(t, snapshotLedgerSlot-iotago.SlotIndex(i), diff.Slot)
		}
	}
}
