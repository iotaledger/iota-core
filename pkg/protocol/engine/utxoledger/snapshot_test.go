package utxoledger_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestOutput_SnapshotBytes(t *testing.T) {
	outputID := iotago_tpkg.RandOutputID(2)
	blockID := iotago_tpkg.RandBlockID()
	txID := iotago_tpkg.RandTransactionID()
	slotBooked := iotago_tpkg.RandSlot()
	iotaOutput := iotago_tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.ZeroCostTestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	proof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)

	proofBytes, err := proof.Bytes()
	require.NoError(t, err)

	output := utxoledger.NewOutput(iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes, proof, proofBytes)

	snapshotBytes := output.SnapshotBytes()

	readOffset := 0
	require.Equal(t, outputID[:], snapshotBytes[readOffset:readOffset+iotago.OutputIDLength], "outputID not equal")
	readOffset += iotago.OutputIDLength
	require.Equal(t, blockID[:], snapshotBytes[readOffset:readOffset+iotago.BlockIDLength], "blockID not equal")
	readOffset += iotago.BlockIDLength
	require.Equal(t, slotBooked, lo.Return1(iotago.SlotIndexFromBytes(snapshotBytes[readOffset:readOffset+iotago.SlotIndexLength])), "slotBooked not equal")
	readOffset += iotago.SlotIndexLength
	require.Equal(t, uint32(len(iotaOutputBytes)), binary.LittleEndian.Uint32(snapshotBytes[readOffset:readOffset+4]), "output bytes length")
	readOffset += 4
	require.Equal(t, iotaOutputBytes, snapshotBytes[readOffset:readOffset+len(iotaOutputBytes)], "output bytes not equal")
	readOffset += len(iotaOutputBytes)
	require.Equal(t, uint32(len(proofBytes)), binary.LittleEndian.Uint32(snapshotBytes[readOffset:readOffset+4]), "proof bytes length")
	readOffset += 4
	require.Equal(t, proofBytes, snapshotBytes[readOffset:readOffset+len(proofBytes)], "proof bytes not equal")
}

func TestOutputFromSnapshotReader(t *testing.T) {
	txID := iotago_tpkg.RandTransactionID()
	outputID := iotago_tpkg.RandOutputID(2)
	blockID := iotago_tpkg.RandBlockID()
	slotBooked := iotago_tpkg.RandSlot()
	iotaOutput := iotago_tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.ZeroCostTestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)
	outputProofBytes, err := outputProof.Bytes()
	require.NoError(t, err)

	output := utxoledger.NewOutput(iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes, outputProof, outputProofBytes)
	snapshotBytes := output.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readOutput, err := utxoledger.OutputFromSnapshotReader(buf, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
	require.NoError(t, err)

	require.Equal(t, output, readOutput)
}

func TestSpent_SnapshotBytes(t *testing.T) {
	txID := iotago_tpkg.RandTransactionID()
	outputID := iotago_tpkg.RandOutputID(2)
	blockID := iotago_tpkg.RandBlockID()
	slotBooked := iotago_tpkg.RandSlot()
	iotaOutput := iotago_tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.ZeroCostTestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)
	outputProofBytes, err := outputProof.Bytes()
	require.NoError(t, err)

	output := utxoledger.NewOutput(iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes, outputProof, outputProofBytes)
	outputSnapshotBytes := output.SnapshotBytes()

	transactionID := iotago_tpkg.RandTransactionID()
	slotSpent := iotago_tpkg.RandSlot()
	spent := utxoledger.NewSpent(output, transactionID, slotSpent)

	snapshotBytes := spent.SnapshotBytes()

	readOffset := 0
	require.Equal(t, outputSnapshotBytes, snapshotBytes[readOffset:readOffset+len(outputSnapshotBytes)], "output bytes not equal")
	readOffset += len(outputSnapshotBytes)
	require.Equal(t, transactionID[:], snapshotBytes[readOffset:readOffset+iotago.TransactionIDLength], "transactionID not equal")
}

func TestSpentFromSnapshotReader(t *testing.T) {
	txID := iotago_tpkg.RandTransactionID()
	outputID := iotago_tpkg.RandOutputID(2)
	blockID := iotago_tpkg.RandBlockID()
	slotBooked := iotago_tpkg.RandSlot()
	iotaOutput := iotago_tpkg.RandOutput(iotago.OutputBasic)
	iotaOutputBytes, err := iotago_tpkg.ZeroCostTestAPI.Encode(iotaOutput)
	require.NoError(t, err)

	outputProof, err := iotago.NewOutputIDProof(iotago_tpkg.ZeroCostTestAPI, txID.Identifier(), txID.Slot(), iotago.TxEssenceOutputs{iotaOutput}, 0)
	require.NoError(t, err)
	outputProofBytes, err := outputProof.Bytes()
	require.NoError(t, err)

	output := utxoledger.NewOutput(iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), outputID, blockID, slotBooked, iotaOutput, iotaOutputBytes, outputProof, outputProofBytes)

	transactionID := iotago_tpkg.RandTransactionID()
	slotSpent := iotago_tpkg.RandSlot()
	spent := utxoledger.NewSpent(output, transactionID, slotSpent)

	snapshotBytes := spent.SnapshotBytes()

	buf := bytes.NewReader(snapshotBytes)
	readSpent, err := utxoledger.SpentFromSnapshotReader(buf, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), slotSpent)
	require.NoError(t, err)

	require.Equal(t, spent, readSpent)
}

func TestReadSlotDiffToSnapshotReader(t *testing.T) {
	slot := iotago_tpkg.RandSlot()
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

	writer := stream.NewByteBuffer()
	err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	reader := writer.Reader()
	readSlotDiff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
	require.NoError(t, err)

	require.Equal(t, slotDiff.Slot, readSlotDiff.Slot)
	tpkg.EqualOutputs(t, slotDiff.Outputs, readSlotDiff.Outputs)
	tpkg.EqualSpents(t, slotDiff.Spents, readSlotDiff.Spents)
}

func TestWriteSlotDiffToSnapshotWriter(t *testing.T) {
	slot := iotago_tpkg.RandSlot()
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

	writer := stream.NewByteBuffer()
	err := utxoledger.WriteSlotDiffToSnapshotWriter(writer, slotDiff)
	require.NoError(t, err)

	reader := writer.Reader()

	var readSlot iotago.SlotIndex
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &readSlot))
	require.Equal(t, slot, readSlot)

	var createdCount uint32
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &createdCount))
	require.Equal(t, uint32(len(slotDiff.Outputs)), createdCount)

	var snapshotOutputs utxoledger.Outputs
	for i := 0; i < len(slotDiff.Outputs); i++ {
		readOutput, err := utxoledger.OutputFromSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
		require.NoError(t, err)
		snapshotOutputs = append(snapshotOutputs, readOutput)
	}

	tpkg.EqualOutputs(t, slotDiff.Outputs, snapshotOutputs)

	var consumedCount uint32
	require.NoError(t, binary.Read(reader, binary.LittleEndian, &consumedCount))
	require.Equal(t, uint32(len(slotDiff.Spents)), consumedCount)

	var snapshotSpents utxoledger.Spents
	for i := 0; i < len(slotDiff.Spents); i++ {
		readSpent, err := utxoledger.SpentFromSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI), readSlot)
		require.NoError(t, err)
		snapshotSpents = append(snapshotSpents, readSpent)
	}

	tpkg.EqualSpents(t, slotDiff.Spents, snapshotSpents)
}

func TestManager_Import(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := utxoledger.New(mapDB, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

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
	require.NoError(t, lo.Return2(manager.ApplyDiff(1,
		utxoledger.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1),
		})))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerSlot)

	mapDBAtSlot1 := mapdb.NewMapDB()
	require.NoError(t, kvstore.Copy(mapDB, mapDBAtSlot1))

	require.NoError(t, lo.Return2(manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		})))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerSlot)

	// Test exporting and importing at the current slot 2
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.Reader()

		importedSlot2 := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
		require.NoError(t, importedSlot2.Import(reader))

		require.Equal(t, iotago.SlotIndex(2), lo.PanicOnErr(importedSlot2.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(manager.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot2.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at slot 1
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.Reader()

		importedSlot1 := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
		require.NoError(t, importedSlot1.Import(reader))

		managerAtSlot1 := utxoledger.New(mapDBAtSlot1, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(importedSlot1.ReadLedgerSlot()))
		require.Equal(t, iotago.SlotIndex(1), lo.PanicOnErr(managerAtSlot1.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(managerAtSlot1.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot1.LedgerStateSHA256Sum()))
	}

	// Test exporting and importing at slot 0
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.Reader()

		importedSlot0 := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
		require.NoError(t, importedSlot0.Import(reader))

		managerAtSlot0 := utxoledger.New(mapDBAtSlot0, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(importedSlot0.ReadLedgerSlot()))
		require.Equal(t, iotago.SlotIndex(0), lo.PanicOnErr(managerAtSlot0.ReadLedgerSlot()))
		require.Equal(t, lo.PanicOnErr(managerAtSlot0.LedgerStateSHA256Sum()), lo.PanicOnErr(importedSlot0.LedgerStateSHA256Sum()))
	}
}

func TestManager_Export(t *testing.T) {
	mapDB := mapdb.NewMapDB()
	manager := utxoledger.New(mapDB, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

	output1 := tpkg.RandLedgerStateOutput()

	require.NoError(t, manager.AddGenesisUnspentOutput(output1))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))
	require.NoError(t, manager.AddGenesisUnspentOutput(tpkg.RandLedgerStateOutput()))

	output2 := tpkg.RandLedgerStateOutput()
	require.NoError(t, lo.Return2(manager.ApplyDiff(1,
		utxoledger.Outputs{
			output2,
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output1, 1),
		})))

	ledgerSlot, err := manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(1), ledgerSlot)

	require.NoError(t, lo.Return2(manager.ApplyDiff(2,
		utxoledger.Outputs{
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
			tpkg.RandLedgerStateOutput(),
		}, utxoledger.Spents{
			tpkg.RandLedgerStateSpentWithOutput(output2, 2),
		})))

	ledgerSlot, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, iotago.SlotIndex(2), ledgerSlot)

	// Test exporting at the current slot 2
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 2))

		reader := writer.Reader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		// Compare the snapshot outputs with our current ledger state
		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		var slotDiffCount uint32
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint32(0), slotDiffCount)
	}

	// Test exporting at slot 1
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 1))

		reader := writer.Reader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		var slotDiffCount uint32
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint32(1), slotDiffCount)

		for i := uint32(0); i < slotDiffCount; i++ {
			diff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerSlot-iotago.SlotIndex(i), diff.Slot)
		}
	}

	// Test exporting at slot 0
	{
		writer := stream.NewByteBuffer()
		require.NoError(t, manager.Export(writer, 0))

		reader := writer.Reader()

		var snapshotLedgerSlot iotago.SlotIndex
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &snapshotLedgerSlot))
		require.Equal(t, iotago.SlotIndex(2), snapshotLedgerSlot)

		var outputCount uint64
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &outputCount))
		require.Equal(t, uint64(8), outputCount)

		var snapshotOutputs utxoledger.Outputs
		for i := uint64(0); i < outputCount; i++ {
			output, err := utxoledger.OutputFromSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
			require.NoError(t, err)
			snapshotOutputs = append(snapshotOutputs, output)
		}

		unspentOutputs, err := manager.UnspentOutputs()
		require.NoError(t, err)

		tpkg.EqualOutputs(t, unspentOutputs, snapshotOutputs)

		var slotDiffCount uint32
		require.NoError(t, binary.Read(reader, binary.LittleEndian, &slotDiffCount))
		require.Equal(t, uint32(2), slotDiffCount)

		for i := uint32(0); i < slotDiffCount; i++ {
			diff, err := utxoledger.ReadSlotDiffToSnapshotReader(reader, iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))
			require.NoError(t, err)
			require.Equal(t, snapshotLedgerSlot-iotago.SlotIndex(i), diff.Slot)
		}
	}
}
