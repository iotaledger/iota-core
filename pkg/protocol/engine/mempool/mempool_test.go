package mempool

import (
	"fmt"
	"testing"
	"time"

	"iota-core/pkg/types"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func mockedVM(inputTransaction types.Transaction, inputs []types.Output, gasLimit ...uint64) (outputs []types.Output, err error) {
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

func TestMemPool(t *testing.T) {
	workerGroup := workerpool.NewGroup(t.Name())

	genesisOutput := newMockedOutput(tpkg.RandTransactionID(), 0)

	ledgerInstance := newMockedLedger()
	ledgerInstance.unspentOutputs[genesisOutput.ID()] = genesisOutput

	memPool := New(mockedVM, ledgerInstance, workerGroup)
	memPool.cachedOutputs.Set(genesisOutput.ID(), &OutputMetadata{ID: genesisOutput.ID(), output: genesisOutput, Spenders: advancedset.New[*TransactionMetadata]()})

	memPool.Events().TransactionSolid.Hook(func(metadata types.TransactionMetadata) {
		fmt.Println("TransactionBooked", metadata.ID())
	})

	require.NoError(t, memPool.ProcessTransaction(newMockedTransaction(1, genesisOutput)))

	time.Sleep(5 * time.Second)
}

type mockedTransaction struct {
	id          types.TransactionID
	inputs      []types.Input
	outputCount uint16
}

func newMockedTransaction(outputCount uint16, inputs ...types.Input) *mockedTransaction {
	return &mockedTransaction{
		id:          tpkg.RandTransactionID(),
		inputs:      inputs,
		outputCount: outputCount,
	}
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

type mockedOutput struct {
	id types.OutputID
}

func newMockedOutput(transactionID types.TransactionID, index uint16) *mockedOutput {
	return &mockedOutput{id: iotago.OutputIDFromTransactionIDAndIndex(transactionID, index)}
}

func (m mockedOutput) ID() types.OutputID {
	return m.id
}

func (m mockedOutput) String() string {
	return "MockedOutput(" + m.id.ToHex() + ")"
}

type mockedLedger struct {
	unspentOutputs map[types.OutputID]types.Output
}

func newMockedLedger() *mockedLedger {
	return &mockedLedger{
		unspentOutputs: make(map[types.OutputID]types.Output),
	}
}

func (m mockedLedger) Output(id types.OutputID) (output types.Output, exists bool) {
	output, exists = m.unspentOutputs[id]

	return output, exists
}

var _ types.Ledger = &mockedLedger{}
