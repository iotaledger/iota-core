package mempool

import (
	"testing"

	"iota-core/pkg/types"

	"github.com/stretchr/testify/require"
)

func TestMemPool(t *testing.T) {
	mockedLedger := newMockedLedger()

	memPool := New(mockedLedger)
	require.NoError(t, memPool.ProcessTransaction(newMockedTransaction()))
}

type mockedTransaction struct {
	id     types.TransactionID
	inputs []types.Input
}

func newMockedTransaction() *mockedTransaction {
	return &mockedTransaction{}
}

func (m mockedTransaction) ID() (types.TransactionID, error) {
	return m.id, nil
}

func (m mockedTransaction) Inputs() ([]types.Input, error) {
	return m.inputs, nil
}

func (m mockedTransaction) String() string {
	return "MockedTransaction(" + m.id.String() + ")"
}

var _ types.Transaction = &mockedTransaction{}

type mockedLedger struct {
	unspentOutputs map[types.OutputID]*OutputMetadata
}

func newMockedLedger() *mockedLedger {
	return &mockedLedger{
		unspentOutputs: make(map[types.OutputID]*OutputMetadata),
	}
}

func (m mockedLedger) Output(id types.OutputID) (output types.Output, exists bool) {
	// TODO implement me
	panic("implement me")
}

var _ types.Ledger = &mockedLedger{}
