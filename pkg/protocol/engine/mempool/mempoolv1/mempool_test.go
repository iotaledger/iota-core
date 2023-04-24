package mempoolv1

import (
	"context"
	"fmt"
	"testing"
	"time"

	types2 "iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/ledger/mockedleger"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func mockedVM(inputTransaction vm.StateTransition, inputs []vm.State, ctx context.Context) (outputs []vm.State, err error) {
	transaction, ok := inputTransaction.(*mockedTransaction)
	if !ok {
		return nil, xerrors.Errorf("invalid transaction type in MockedVM")
	}

	for i := uint16(0); i < transaction.outputCount; i++ {
		id, err := transaction.ID()
		if err != nil {
			return nil, err
		}

		outputs = append(outputs, newMockedOutput(id, i))
	}

	return outputs, nil
}

type MockedStateReferenceResolver struct {
	states          *mockedleger.StateResolver
	stateIDsByAlias map[string]iotago.OutputID
}

func NewMockedStateReferenceResolver(initialElements map[string]vm.State) *MockedStateReferenceResolver {
	m := &MockedStateReferenceResolver{
		states:          mockedleger.NewStateResolver(),
		stateIDsByAlias: make(map[string]iotago.OutputID),
	}

	for alias, state := range initialElements {
		m.stateIDsByAlias[alias] = state.ID()
		m.states.AddState(state)
	}

	return m
}

func (m *MockedStateReferenceResolver) ResolveReference(reference mempool.StateReference) *promise.Promise[vm.State] {
	switch reference := reference.(type) {
	case mockedLedgerStateReference:
		return m.states.ResolveState(reference.ReferencedStateID())
	default:
		panic("invalid reference type")
	}
}

func (m *MockedStateReferenceResolver) StateID(alias string) iotago.OutputID {
	return m.stateIDsByAlias[alias]
}

func TestMemPool(t *testing.T) {
	mockedLedgerStateReferenceResolver := NewMockedStateReferenceResolver(map[string]vm.State{
		"genesis": newMockedOutput(tpkg.RandTransactionID(), 0),
	})

	memPool := New(mockedVM, mockedLedgerStateReferenceResolver.ResolveReference, workerpool.NewGroup(t.Name()))

	memPool.Events().TransactionStored.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionStored", metadata.ID())
	})

	memPool.Events().TransactionSolid.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionSolid", metadata.ID())
	})

	memPool.Events().TransactionExecuted.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionExecuted", metadata.ID())
	})

	memPool.Events().TransactionBooked.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionBooked", metadata.ID())
	})

	require.NoError(t, memPool.ProcessTransaction(newMockedTransaction(1, mockedLedgerStateReference(mockedLedgerStateReferenceResolver.StateID("genesis")))))

	time.Sleep(5 * time.Second)
}

type mockedLedgerStateReference iotago.OutputID

func (m mockedLedgerStateReference) ReferencedStateID() iotago.OutputID {
	return iotago.OutputID(m)
}

type mockedTransaction struct {
	id          iotago.TransactionID
	inputs      []mempool.StateReference
	outputCount uint16
}

func newMockedTransaction(outputCount uint16, inputs ...mempool.StateReference) *mockedTransaction {
	return &mockedTransaction{
		id:          tpkg.RandTransactionID(),
		inputs:      inputs,
		outputCount: outputCount,
	}
}

func (m mockedTransaction) ID() (iotago.TransactionID, error) {
	return m.id, nil
}

func (m mockedTransaction) Inputs() ([]mempool.StateReference, error) {
	return m.inputs, nil
}

func (m mockedTransaction) String() string {
	return "MockedTransaction(" + m.id.String() + ")"
}

var _ vm.StateTransition = &mockedTransaction{}

type mockedOutput struct {
	id iotago.OutputID
}

func newMockedOutput(transactionID iotago.TransactionID, index uint16) *mockedOutput {
	return &mockedOutput{id: iotago.OutputIDFromTransactionIDAndIndex(transactionID, index)}
}

func (m mockedOutput) ID() iotago.OutputID {
	return m.id
}

func (m mockedOutput) String() string {
	return "MockedOutput(" + m.id.ToHex() + ")"
}

type mockedLedger struct {
	unspentOutputs map[iotago.OutputID]vm.State
}

func newMockedLedger() *mockedLedger {
	return &mockedLedger{
		unspentOutputs: make(map[iotago.OutputID]vm.State),
	}
}

func (m mockedLedger) Output(id iotago.OutputID) (output vm.State, exists bool) {
	output, exists = m.unspentOutputs[id]

	return output, exists
}

var _ types2.Ledger = &mockedLedger{}
