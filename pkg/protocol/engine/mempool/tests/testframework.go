package mempooltests

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance mempool.MemPool

	stateIDByAlias               map[string]iotago.OutputID
	transactionIDByAlias         map[string]iotago.TransactionID
	transactionStoredTriggered   map[iotago.TransactionID]bool
	transactionSolidTriggered    map[iotago.TransactionID]bool
	transactionExecutedTriggered map[iotago.TransactionID]bool
	transactionBookedTriggered   map[iotago.TransactionID]bool
	test                         *testing.T
	mutex                        sync.RWMutex
}

func NewTestFramework(test *testing.T, instance mempool.MemPool) *TestFramework {
	t := &TestFramework{
		Instance:                     instance,
		stateIDByAlias:               make(map[string]iotago.OutputID),
		transactionIDByAlias:         make(map[string]iotago.TransactionID),
		transactionStoredTriggered:   make(map[iotago.TransactionID]bool),
		transactionSolidTriggered:    make(map[iotago.TransactionID]bool),
		transactionBookedTriggered:   make(map[iotago.TransactionID]bool),
		transactionExecutedTriggered: make(map[iotago.TransactionID]bool),
		test:                         test,
	}

	t.setupHookedEvents()

	return t
}

func (t *TestFramework) StateID(alias string) iotago.OutputID {
	if alias == "genesis" {
		return iotago.OutputID{}
	}

	stateID, exists := t.stateIDByAlias[alias]
	require.True(t.test, exists, "StateID with alias '%s' does not exist", alias)

	return stateID
}

func (t *TestFramework) TransactionID(alias string) iotago.TransactionID {
	transactionID, exists := t.transactionIDByAlias[alias]
	require.True(t.test, exists, "transaction with alias '%s' does not exist", alias)

	return transactionID
}

func (t *TestFramework) StateReference(alias string) mempool.StateReference {
	return LedgerStateReference(t.StateID(alias))
}

func (t *TestFramework) ProcessTransaction(alias string, referencedStates []string, outputCount uint16) error {
	transaction := NewTransaction(outputCount, lo.Map(referencedStates, t.StateReference)...)

	t.registerTransactionAlias(alias, transaction)

	return t.Instance.ProcessTransaction(transaction)
}

func (t *TestFramework) RequireBooked(transactionAliases ...string) {
	t.waitBooked(transactionAliases...)

	t.requireBookedTriggered(transactionAliases...)
	t.requireMarkedBooked(transactionAliases...)
}

func (t *TestFramework) setupHookedEvents() {
	t.Instance.Events().TransactionStored.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionStored with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.IsStored(), "transaction is not marked as stored")

		t.markTransactionStoredTriggered(metadata.ID())
	})

	t.Instance.Events().TransactionSolid.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionSolid with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.IsSolid(), "transaction is not marked as solid")

		t.markTransactionSolidTriggered(metadata.ID())
	})

	t.Instance.Events().TransactionExecuted.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionExecuted with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.IsExecuted(), "transaction is not marked as executed")

		t.markTransactionExecutedTriggered(metadata.ID())
	})

	t.Instance.Events().TransactionBooked.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionBooked with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.IsBooked(), "transaction is not marked as booked")

		t.markTransactionBookedTriggered(metadata.ID())
	})
}

func (t *TestFramework) markTransactionStoredTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.transactionStoredTriggered[id] = true
}

func (t *TestFramework) markTransactionSolidTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.transactionSolidTriggered[id] = true
}

func (t *TestFramework) markTransactionExecutedTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.transactionExecutedTriggered[id] = true
}

func (t *TestFramework) markTransactionBookedTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.transactionBookedTriggered[id] = true
}

func (t *TestFramework) registerTransactionAlias(alias string, transaction *Transaction) {
	t.transactionIDByAlias[alias] = lo.PanicOnErr(transaction.ID())
	t.transactionIDByAlias[alias].RegisterAlias(alias)

	for i := uint16(0); i < transaction.outputCount; i++ {
		t.stateIDByAlias[alias+":"+strconv.Itoa(int(i))] = iotago.OutputIDFromTransactionIDAndIndex(t.transactionIDByAlias[alias], i)
	}
}

func (t *TestFramework) waitBooked(transactionAliases ...string) {
	for _, transactionAlias := range transactionAliases {
		require.Eventuallyf(t.test, func() bool {
			transactionMetadata, transactionMetadataExists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))
			bookedTriggered, _ := t.transactionBookedTriggered[t.TransactionID(transactionAlias)]

			return bookedTriggered || transactionMetadataExists && transactionMetadata.IsBooked()
		}, 10*time.Second, 50*time.Millisecond, "transaction '%s' was not booked in time", transactionAlias)
	}
}

func (t *TestFramework) requireBookedTriggered(transactionAliases ...string) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for _, transactionAlias := range transactionAliases {
		require.True(t.test, t.transactionBookedTriggered[t.TransactionID(transactionAlias)], "transaction '%s' was not booked", transactionAlias)
	}
}

func (t *TestFramework) requireMarkedBooked(transactionAliases ...string) {
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, transactionMetadataExists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))

		require.True(t.test, transactionMetadataExists && transactionMetadata.IsBooked(), "transaction %s was not booked", transactionAlias)
	}
}
