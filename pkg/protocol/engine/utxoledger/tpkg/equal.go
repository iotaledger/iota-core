package tpkg

import (
	"bytes"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

func EqualOutput(t *testing.T, expected *utxoledger.Output, actual *utxoledger.Output) {
	t.Helper()

	require.Equal(t, expected.OutputID(), actual.OutputID())
	require.Equal(t, expected.BlockID(), actual.BlockID())
	require.Equal(t, expected.SlotBooked(), actual.SlotBooked())
	require.Equal(t, expected.SlotCreated(), actual.SlotCreated())
	require.Equal(t, expected.OutputType(), actual.OutputType())

	var expectedOwner iotago.Address
	switch output := expected.Output().(type) {
	case iotago.OwnerTransitionIndependentOutput:
		expectedOwner = output.Owner()
	case iotago.OwnerTransitionDependentOutput:
		expectedOwner = output.ChainID().ToAddress()
	default:
		require.Fail(t, "unsupported output type")
	}

	var actualOwner iotago.Address
	switch output := actual.Output().(type) {
	case iotago.OwnerTransitionIndependentOutput:
		actualOwner = output.Owner()
	case iotago.OwnerTransitionDependentOutput:
		actualOwner = output.ChainID().ToAddress()
	default:
		require.Fail(t, "unsupported output type")
	}

	require.NotNil(t, expectedOwner)
	require.NotNil(t, actualOwner)
	require.True(t, expectedOwner.Equal(actualOwner))
	require.Equal(t, expected.BaseTokenAmount(), actual.BaseTokenAmount())
	require.EqualValues(t, expected.Output(), actual.Output())
}

func EqualSpent(t *testing.T, expected *utxoledger.Spent, actual *utxoledger.Spent) {
	t.Helper()

	require.Equal(t, expected.OutputID(), actual.OutputID())
	require.Equal(t, expected.TransactionIDSpent(), actual.TransactionIDSpent())
	require.Equal(t, expected.SlotSpent(), actual.SlotSpent())
	EqualOutput(t, expected.Output(), actual.Output())
}

func EqualOutputs(t *testing.T, expected utxoledger.Outputs, actual utxoledger.Outputs) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	// Sort Outputs by output ID.
	sort.Slice(expected, func(i int, j int) bool {
		iOutputID := expected[i].OutputID()
		jOutputID := expected[j].OutputID()

		return bytes.Compare(iOutputID[:], jOutputID[:]) == -1
	})
	sort.Slice(actual, func(i int, j int) bool {
		iOutputID := actual[i].OutputID()
		jOutputID := actual[j].OutputID()

		return bytes.Compare(iOutputID[:], jOutputID[:]) == -1
	})

	for i := range expected {
		EqualOutput(t, expected[i], actual[i])
	}
}

func EqualSpents(t *testing.T, expected utxoledger.Spents, actual utxoledger.Spents) {
	t.Helper()

	require.Equal(t, len(expected), len(actual))

	// Sort Spents by output ID.
	sort.Slice(expected, func(i int, j int) bool {
		iOutputID := expected[i].OutputID()
		jOutputID := expected[j].OutputID()

		return bytes.Compare(iOutputID[:], jOutputID[:]) == -1
	})
	sort.Slice(actual, func(i int, j int) bool {
		iOutputID := actual[i].OutputID()
		jOutputID := actual[j].OutputID()

		return bytes.Compare(iOutputID[:], jOutputID[:]) == -1
	})

	for i := range len(expected) {
		EqualSpent(t, expected[i], actual[i])
	}
}
