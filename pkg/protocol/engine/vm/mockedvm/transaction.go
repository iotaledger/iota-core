package mockedvm

import (
	"strconv"
	"sync"
	"sync/atomic"

	"iota-core/pkg/iotago"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/vm"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix/model"
)

var MockedTransactionType iotago.PayloadType

// MockedTransaction is the type that is used to describe instructions how to modify the ledger state for MockedVM.
type MockedTransaction struct {
	model.Immutable[MockedTransaction, *MockedTransaction, mockedTransaction] `serix:"0"`

	id      iotago.TransactionID
	idMutex sync.RWMutex
}

type mockedTransaction struct {
	// Inputs contains the list of MockedInput objects that address the consumed Outputs.
	Inputs []*MockedInput `serix:"0,lengthPrefixType=uint16"`

	// OutputCount contains the number of Outputs that this MockedTransaction creates.
	OutputCount uint16 `serix:"1"`

	// UniqueEssence contains a unique value for each created MockedTransaction to ensure a unique TransactionID.
	UniqueEssence uint64 `serix:"2"`
}

// NewMockedTransaction creates a new MockedTransaction with the given inputs and specified outputCount.
// A unique essence is simulated by an atomic counter, incremented globally for each MockedTransaction created.
func NewMockedTransaction(inputs []*MockedInput, outputCount uint16) (tx *MockedTransaction) {
	m := model.NewImmutable[MockedTransaction](&mockedTransaction{
		Inputs:        inputs,
		OutputCount:   outputCount,
		UniqueEssence: atomic.AddUint64(&_uniqueEssenceCounter, 1),
	})

	idSeed := strconv.Itoa(int(m.M.UniqueEssence)) + strconv.Itoa(int(m.M.OutputCount))
	for _, input := range m.M.Inputs {
		idSeed += input.ID().ToHex()
	}
	m.id = iotago.IdentifierFromData([]byte(idSeed))

	return m
}

func (m *MockedTransaction) ID() (iotago.TransactionID, error) {
	return m.id, nil
}

// Inputs returns the inputs of the Transaction.
func (m *MockedTransaction) Inputs() ([]mempool.StateReference, error) {
	return lo.Map(m.M.Inputs, func(input *MockedInput) mempool.StateReference {
		return input
	}), nil
}

// PayloadType returns the type of the Transaction.
func (m *MockedTransaction) PayloadType() iotago.PayloadType {
	return MockedTransactionType
}

// Size returns the size of the Transaction.
func (m *MockedTransaction) Size() int {
	return 1
}

// Size returns the size of the Transaction.
func (m *MockedTransaction) String() string {
	return ""
}

// code contract (make sure the struct implements all required methods).
var (
	_ vm.StateTransition = new(MockedTransaction)
	_ iotago.Payload     = new(MockedTransaction)
)

// _uniqueEssenceCounter contains a counter that is used to generate unique TransactionIDs.
var _uniqueEssenceCounter uint64
