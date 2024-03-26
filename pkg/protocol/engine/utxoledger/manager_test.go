//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package utxoledger_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/fjl/memsize"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestConfirmationApplyAndRollbackToEmptyLedger(t *testing.T) {
	manager := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAccount),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAnchor),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
	}

	slot := iotago.SlotIndex(756)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], slot),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], slot),
	}

	require.NoError(t, lo.Return2(manager.ApplyDiffWithoutLocking(slot, outputs, spents)))

	require.NotEqual(t, manager.StateTreeRoot(), iotago.Identifier{})
	require.True(t, manager.CheckStateTree())

	var outputCount int
	require.NoError(t, manager.ForEachOutput(func(_ *utxoledger.Output) bool {
		outputCount++

		return true
	}))
	require.Equal(t, 8, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(_ *utxoledger.Output) bool {
		unspentCount++

		return true
	}))
	require.Equal(t, 6, unspentCount)

	var spentCount int
	require.NoError(t, manager.ForEachSpentOutput(func(_ *utxoledger.Spent) bool {
		spentCount++

		return true
	}))
	require.Equal(t, 2, spentCount)

	require.NoError(t, manager.RollbackDiffWithoutLocking(slot, outputs, spents))

	require.NoError(t, manager.ForEachOutput(func(_ *utxoledger.Output) bool {
		require.Fail(t, "should not be called")

		return true
	}))

	require.NoError(t, manager.ForEachUnspentOutput(func(_ *utxoledger.Output) bool {
		require.Fail(t, "should not be called")

		return true
	}))

	require.NoError(t, manager.ForEachSpentOutput(func(_ *utxoledger.Spent) bool {
		require.Fail(t, "should not be called")

		return true
	}))

	require.Equal(t, manager.StateTreeRoot(), iotago.Identifier{})
	require.True(t, manager.CheckStateTree())
}

func TestConfirmationApplyAndRollbackToPreviousLedger(t *testing.T) {
	manager := utxoledger.New(mapdb.NewMapDB(), iotago.SingleVersionProvider(iotago_tpkg.ZeroCostTestAPI))

	previousOutputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent on 2nd confirmation
	}

	previousMsIndex := iotago.SlotIndex(48)
	previousSpents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[1], previousMsIndex),
	}
	require.NoError(t, lo.Return2(manager.ApplyDiffWithoutLocking(previousMsIndex, previousOutputs, previousSpents)))

	require.True(t, manager.CheckStateTree())

	ledgerIndex, err := manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, previousMsIndex, ledgerIndex)

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAccount),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAnchor),
	}

	index := iotago.SlotIndex(49)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[2], index),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index),
	}
	require.NoError(t, lo.Return2(manager.ApplyDiffWithoutLocking(index, outputs, spents)))

	require.True(t, manager.CheckStateTree())

	ledgerIndex, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, index, ledgerIndex)

	// Prepare values to check
	outputByOutputID := make(map[string]struct{})
	unspentByOutputID := make(map[string]struct{})
	for _, output := range previousOutputs {
		outputByOutputID[output.MapKey()] = struct{}{}
		unspentByOutputID[output.MapKey()] = struct{}{}
	}
	for _, output := range outputs {
		outputByOutputID[output.MapKey()] = struct{}{}
		unspentByOutputID[output.MapKey()] = struct{}{}
	}

	spentByOutputID := make(map[string]struct{})
	for _, spent := range previousSpents {
		spentByOutputID[spent.MapKey()] = struct{}{}
		delete(unspentByOutputID, spent.MapKey())
	}
	for _, spent := range spents {
		spentByOutputID[spent.MapKey()] = struct{}{}
		delete(unspentByOutputID, spent.MapKey())
	}

	var outputCount int
	require.NoError(t, manager.ForEachOutput(func(output *utxoledger.Output) bool {
		outputCount++
		_, has := outputByOutputID[output.MapKey()]
		require.True(t, has)
		delete(outputByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, outputByOutputID)
	require.Equal(t, 8, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		unspentCount++
		_, has := unspentByOutputID[output.MapKey()]
		require.True(t, has)
		delete(unspentByOutputID, output.MapKey())

		return true
	}))
	require.Equal(t, 5, unspentCount)
	require.Empty(t, unspentByOutputID)

	var spentCount int
	require.NoError(t, manager.ForEachSpentOutput(func(spent *utxoledger.Spent) bool {
		spentCount++
		_, has := spentByOutputID[spent.MapKey()]
		require.True(t, has)
		delete(spentByOutputID, spent.MapKey())

		return true
	}))
	require.Empty(t, spentByOutputID)
	require.Equal(t, 3, spentCount)

	require.NoError(t, manager.RollbackDiffWithoutLocking(index, outputs, spents))

	require.True(t, manager.CheckStateTree())

	ledgerIndex, err = manager.ReadLedgerSlot()
	require.NoError(t, err)
	require.Equal(t, previousMsIndex, ledgerIndex)

	// Prepare values to check
	outputByOutputID = make(map[string]struct{})
	unspentByOutputID = make(map[string]struct{})
	spentByOutputID = make(map[string]struct{})

	for _, output := range previousOutputs {
		outputByOutputID[output.MapKey()] = struct{}{}
		unspentByOutputID[output.MapKey()] = struct{}{}
	}

	for _, spent := range previousSpents {
		spentByOutputID[spent.MapKey()] = struct{}{}
		delete(unspentByOutputID, spent.MapKey())
	}

	require.NoError(t, manager.ForEachOutput(func(output *utxoledger.Output) bool {
		_, has := outputByOutputID[output.MapKey()]
		require.True(t, has)
		delete(outputByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, outputByOutputID)

	require.NoError(t, manager.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		_, has := unspentByOutputID[output.MapKey()]
		require.True(t, has)
		delete(unspentByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, unspentByOutputID)

	require.NoError(t, manager.ForEachSpentOutput(func(spent *utxoledger.Spent) bool {
		_, has := spentByOutputID[spent.MapKey()]
		require.True(t, has)
		delete(spentByOutputID, spent.MapKey())

		return true
	}))
	require.Empty(t, spentByOutputID)
}

func TestMemLeakStateTree(t *testing.T) {
	t.Skip("This test is not meant to be run in CI, it's for local testing only")

	dbConfig := database.Config{
		Engine:       db.EngineRocksDB,
		Directory:    t.TempDir(),
		Version:      1,
		PrefixHealth: []byte{2},
	}
	rocksDB := database.NewDBInstance(dbConfig, nil)
	kvStore := rocksDB.KVStore()
	stateTree := ads.NewMap[iotago.Identifier](kvStore,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.OutputID.Bytes,
		iotago.OutputIDFromBytes,
		(*stateTreeMetadata).Bytes,
		stateMetadataFromBytes,
	)

	var totalOutputs int
	var allOutputs []*utxoledger.Output
	runTest := func(reInit bool, outputCount int) {
		totalOutputs += outputCount
		fmt.Println(">>> Running with", outputCount, "outputs, reInit:", reInit)
		fmt.Println(">>> Total outputs:", totalOutputs)

		start := time.Now()
		if reInit {
			stateTree = ads.NewMap[iotago.Identifier](kvStore,
				iotago.Identifier.Bytes,
				iotago.IdentifierFromBytes,
				iotago.OutputID.Bytes,
				iotago.OutputIDFromBytes,
				(*stateTreeMetadata).Bytes,
				stateMetadataFromBytes,
			)
		}

		newOutputs := make([]*utxoledger.Output, outputCount)
		{
			for i := 0; i < outputCount; i++ {
				newOutputs[i] = tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic)
				allOutputs = append(allOutputs, newOutputs[i])
			}

			for _, output := range newOutputs {
				if err := stateTree.Set(output.OutputID(), newStateMetadata(output)); err != nil {
					panic(ierrors.Wrapf(err, "failed to set new oputput in state tree, outputID: %s", output.OutputID().ToHex()))
				}
			}

			if err := stateTree.Commit(); err != nil {
				panic(ierrors.Wrap(err, "failed to commit state tree"))
			}
		}

		memConsumptionEnd := memsize.Scan(stateTree)
		fmt.Println(">>> Took: ", time.Since(start).String())
		fmt.Println()
		fmt.Println(">>> Memory:", memConsumptionEnd.Report())

		// Check that all outputs are in the tree
		for _, output := range allOutputs {
			exists, err := stateTree.Has(output.OutputID())
			require.NoError(t, err)
			require.True(t, exists)
		}

		fmt.Printf("----------------------------------------------------------------\n\n")
	}

	runTest(false, 1000000)
	runTest(false, 1000000)
	runTest(false, 1000000)
	runTest(false, 10000)
}

type stateTreeMetadata struct {
	Slot iotago.SlotIndex
}

func newStateMetadata(output *utxoledger.Output) *stateTreeMetadata {
	return &stateTreeMetadata{
		Slot: output.SlotCreated(),
	}
}

func stateMetadataFromBytes(b []byte) (*stateTreeMetadata, int, error) {
	s := new(stateTreeMetadata)

	var err error
	var n int
	s.Slot, n, err = iotago.SlotIndexFromBytes(b)
	if err != nil {
		return nil, 0, err
	}

	return s, n, nil
}

func (s *stateTreeMetadata) Bytes() ([]byte, error) {
	return s.Slot.Bytes()
}
