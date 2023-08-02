//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package utxoledger_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	iotago_tpkg "github.com/iotaledger/iota.go/v4/tpkg"
)

func TestConfirmationApplyAndRollbackToEmptyLedger(t *testing.T) {
	manager := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAccount),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
	}

	index := iotago.SlotIndex(756)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], index),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index),
	}

	require.NoError(t, manager.ApplyDiffWithoutLocking(index, outputs, spents))

	require.NotEqual(t, manager.StateTreeRoot(), iotago.Identifier{})
	require.True(t, manager.CheckStateTree())

	var outputCount int
	require.NoError(t, manager.ForEachOutput(func(_ *utxoledger.Output) bool {
		outputCount++

		return true
	}))
	require.Equal(t, 7, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(_ *utxoledger.Output) bool {
		unspentCount++

		return true
	}))
	require.Equal(t, 5, unspentCount)

	var spentCount int
	require.NoError(t, manager.ForEachSpentOutput(func(_ *utxoledger.Spent) bool {
		spentCount++

		return true
	}))
	require.Equal(t, 2, spentCount)

	require.NoError(t, manager.RollbackDiffWithoutLocking(index, outputs, spents))

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
	manager := utxoledger.New(mapdb.NewMapDB(), api.SingleVersionProvider(iotago_tpkg.TestAPI))

	previousOutputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent on 2nd confirmation
	}

	previousMsIndex := iotago.SlotIndex(48)
	previousSpents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[1], previousMsIndex),
	}
	require.NoError(t, manager.ApplyDiffWithoutLocking(previousMsIndex, previousOutputs, previousSpents))

	require.True(t, manager.CheckStateTree())

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, previousMsIndex, ledgerIndex)

	outputs := utxoledger.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAccount),
	}

	index := iotago.SlotIndex(49)

	spents := utxoledger.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[2], index),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index),
	}
	require.NoError(t, manager.ApplyDiffWithoutLocking(index, outputs, spents))

	require.True(t, manager.CheckStateTree())

	ledgerIndex, err = manager.ReadLedgerIndex()
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
	require.Equal(t, 7, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(output *utxoledger.Output) bool {
		unspentCount++
		_, has := unspentByOutputID[output.MapKey()]
		require.True(t, has)
		delete(unspentByOutputID, output.MapKey())

		return true
	}))
	require.Equal(t, 4, unspentCount)
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

	ledgerIndex, err = manager.ReadLedgerIndex()
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
