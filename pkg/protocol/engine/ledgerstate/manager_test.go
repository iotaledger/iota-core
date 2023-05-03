//nolint:forcetypeassert,varnamelen,revive,exhaustruct // we don't care about these linters in test cases
package ledgerstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate/tpkg"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestConfirmationApplyAndRollbackToEmptyLedger(t *testing.T) {
	manager := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)

	outputs := ledgerstate.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAlias),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
	}

	index := iotago.SlotIndex(756)

	spents := ledgerstate.Spents{
		tpkg.RandLedgerStateSpentWithOutput(outputs[3], index, tpkg.RandTimestamp()),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index, tpkg.RandTimestamp()),
	}

	require.NoError(t, manager.ApplyConfirmationWithoutLocking(index, outputs, spents))

	var outputCount int
	require.NoError(t, manager.ForEachOutput(func(_ *ledgerstate.Output) bool {
		outputCount++

		return true
	}))
	require.Equal(t, 7, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(_ *ledgerstate.Output) bool {
		unspentCount++

		return true
	}))
	require.Equal(t, 5, unspentCount)

	var spentCount int
	require.NoError(t, manager.ForEachSpentOutput(func(_ *ledgerstate.Spent) bool {
		spentCount++

		return true
	}))
	require.Equal(t, 2, spentCount)

	require.NoError(t, manager.RollbackConfirmationWithoutLocking(index, outputs, spents))

	require.NoError(t, manager.ForEachOutput(func(_ *ledgerstate.Output) bool {
		require.Fail(t, "should not be called")

		return true
	}))

	require.NoError(t, manager.ForEachUnspentOutput(func(_ *ledgerstate.Output) bool {
		require.Fail(t, "should not be called")

		return true
	}))

	require.NoError(t, manager.ForEachSpentOutput(func(_ *ledgerstate.Spent) bool {
		require.Fail(t, "should not be called")

		return true
	}))
}

func TestConfirmationApplyAndRollbackToPreviousLedger(t *testing.T) {
	manager := ledgerstate.New(mapdb.NewMapDB(), tpkg.API)

	previousOutputs := ledgerstate.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputNFT),   // spent on 2nd confirmation
	}

	previousMsIndex := iotago.SlotIndex(48)
	previousMsTimestamp := tpkg.RandTimestamp()
	previousSpents := ledgerstate.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[1], previousMsIndex, previousMsTimestamp),
	}
	require.NoError(t, manager.ApplyConfirmationWithoutLocking(previousMsIndex, previousOutputs, previousSpents))

	ledgerIndex, err := manager.ReadLedgerIndex()
	require.NoError(t, err)
	require.Equal(t, previousMsIndex, ledgerIndex)

	outputs := ledgerstate.Outputs{
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputFoundry),
		tpkg.RandLedgerStateOutputWithType(iotago.OutputBasic), // spent
		tpkg.RandLedgerStateOutputWithType(iotago.OutputAlias),
	}

	index := iotago.SlotIndex(49)

	spents := ledgerstate.Spents{
		tpkg.RandLedgerStateSpentWithOutput(previousOutputs[2], index, tpkg.RandTimestamp()),
		tpkg.RandLedgerStateSpentWithOutput(outputs[2], index, tpkg.RandTimestamp()),
	}
	require.NoError(t, manager.ApplyConfirmationWithoutLocking(index, outputs, spents))

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
	require.NoError(t, manager.ForEachOutput(func(output *ledgerstate.Output) bool {
		outputCount++
		_, has := outputByOutputID[output.MapKey()]
		require.True(t, has)
		delete(outputByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, outputByOutputID)
	require.Equal(t, 7, outputCount)

	var unspentCount int
	require.NoError(t, manager.ForEachUnspentOutput(func(output *ledgerstate.Output) bool {
		unspentCount++
		_, has := unspentByOutputID[output.MapKey()]
		require.True(t, has)
		delete(unspentByOutputID, output.MapKey())

		return true
	}))
	require.Equal(t, 4, unspentCount)
	require.Empty(t, unspentByOutputID)

	var spentCount int
	require.NoError(t, manager.ForEachSpentOutput(func(spent *ledgerstate.Spent) bool {
		spentCount++
		_, has := spentByOutputID[spent.MapKey()]
		require.True(t, has)
		delete(spentByOutputID, spent.MapKey())

		return true
	}))
	require.Empty(t, spentByOutputID)
	require.Equal(t, 3, spentCount)

	require.NoError(t, manager.RollbackConfirmationWithoutLocking(index, outputs, spents))

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

	require.NoError(t, manager.ForEachOutput(func(output *ledgerstate.Output) bool {
		_, has := outputByOutputID[output.MapKey()]
		require.True(t, has)
		delete(outputByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, outputByOutputID)

	require.NoError(t, manager.ForEachUnspentOutput(func(output *ledgerstate.Output) bool {
		_, has := unspentByOutputID[output.MapKey()]
		require.True(t, has)
		delete(unspentByOutputID, output.MapKey())

		return true
	}))
	require.Empty(t, unspentByOutputID)

	require.NoError(t, manager.ForEachSpentOutput(func(spent *ledgerstate.Spent) bool {
		_, has := spentByOutputID[spent.MapKey()]
		require.True(t, has)
		delete(spentByOutputID, spent.MapKey())

		return true
	}))
	require.Empty(t, spentByOutputID)
}
