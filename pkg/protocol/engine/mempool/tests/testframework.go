package mempooltests

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance mempool.MemPool[vote.MockedPower]

	stateIDByAlias               map[string]iotago.OutputID
	transactionByAlias           map[string]mempool.Transaction
	blockIDsByAlias              map[string]iotago.BlockID
	globalStoredEventTriggered   map[iotago.TransactionID]bool
	globalSolidEventTriggered    map[iotago.TransactionID]bool
	globalExecutedEventTriggered map[iotago.TransactionID]bool
	globalBookedEventTriggered   map[iotago.TransactionID]bool
	test                         *testing.T
	mutex                        sync.RWMutex
}

func NewTestFramework(test *testing.T, instance mempool.MemPool[vote.MockedPower]) *TestFramework {
	t := &TestFramework{
		Instance:                     instance,
		stateIDByAlias:               make(map[string]iotago.OutputID),
		transactionByAlias:           make(map[string]mempool.Transaction),
		blockIDsByAlias:              make(map[string]iotago.BlockID),
		globalStoredEventTriggered:   make(map[iotago.TransactionID]bool),
		globalSolidEventTriggered:    make(map[iotago.TransactionID]bool),
		globalBookedEventTriggered:   make(map[iotago.TransactionID]bool),
		globalExecutedEventTriggered: make(map[iotago.TransactionID]bool),
		test:                         test,
	}

	t.setupHookedEvents()

	return t
}

func (t *TestFramework) CreateTransaction(alias string, referencedStates []string, outputCount uint16) {
	// create transaction
	transaction := NewTransaction(outputCount, lo.Map(referencedStates, t.stateReference)...)
	t.transactionByAlias[alias] = transaction

	// register the transaction ID alias
	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to retrieve transaction ID of transaction with alias '%s'", alias)
	transactionID.RegisterAlias(alias)

	// register the aliases for the generated output IDs
	for i := uint16(0); i < transaction.outputCount; i++ {
		t.stateIDByAlias[alias+":"+strconv.Itoa(int(i))] = iotago.OutputIDFromTransactionIDAndIndex(transactionID, i)
	}
}

func (t *TestFramework) MarkAttachmentIncluded(alias string) error {
	return t.Instance.MarkAttachmentIncluded(t.BlockID(alias))
}

func (t *TestFramework) BlockID(alias string) iotago.BlockID {
	blockID, exists := t.blockIDsByAlias[alias]
	require.True(t.test, exists, "block ID with alias '%s' does not exist", alias)

	return blockID
}

func (t *TestFramework) AttachTransactions(transactionAlias ...string) error {
	for _, alias := range transactionAlias {
		if err := t.AttachTransaction(alias, alias, 1); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestFramework) AttachTransaction(transactionAlias, blockAlias string, slotIndex iotago.SlotIndex) error {
	transaction, transactionExists := t.transactionByAlias[transactionAlias]
	require.True(t.test, transactionExists, "transaction with alias '%s' does not exist", transactionAlias)

	t.blockIDsByAlias[blockAlias] = iotago.SlotIdentifierRepresentingData(slotIndex, []byte(blockAlias))

	if _, err := t.Instance.AttachTransaction(transaction, t.blockIDsByAlias[blockAlias]); err != nil {
		return err
	}

	return nil
}

func (t *TestFramework) TransactionMetadata(alias string) (mempool.TransactionWithMetadata, bool) {
	return t.Instance.Transaction(t.TransactionID(alias))
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
	transaction, transactionExists := t.transactionByAlias[alias]
	require.True(t.test, transactionExists, "transaction with alias '%s' does not exist", alias)

	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to retrieve transaction ID of transaction with alias '%s'", alias)

	return transactionID
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

		require.NotNilf(t.test, metadata, "transaction metadata is nil")

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

	t.Instance.Events().TransactionAccepted.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionAccepted with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.WasAccepted(), "transaction is not marked as accepted")

		t.markTransactionStoredTriggered(metadata.ID())
	})
}

func (t *TestFramework) markTransactionStoredTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.globalStoredEventTriggered[id] = true
}

func (t *TestFramework) markTransactionSolidTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.globalSolidEventTriggered[id] = true
}

func (t *TestFramework) markTransactionExecutedTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.globalExecutedEventTriggered[id] = true
}

func (t *TestFramework) markTransactionBookedTriggered(id iotago.TransactionID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.globalBookedEventTriggered[id] = true
}

func (t *TestFramework) stateReference(alias string) ledger.StateReference {
	return ledger.StoredStateReference(t.StateID(alias))
}

func (t *TestFramework) waitBooked(transactionAliases ...string) {
	var allBooked sync.WaitGroup

	allBooked.Add(len(transactionAliases))
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, exists := t.TransactionMetadata(transactionAlias)
		require.True(t.test, exists, "transaction '%s' does not exist", transactionAlias)

		transactionMetadata.OnBooked(allBooked.Done)
	}

	time.Sleep(100 * time.Millisecond)

	allBooked.Wait()
}

func (t *TestFramework) requireBookedTriggered(transactionAliases ...string) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	for _, transactionAlias := range transactionAliases {
		require.True(t.test, t.globalBookedEventTriggered[t.TransactionID(transactionAlias)], "transaction '%s' was not booked", transactionAlias)
	}
}

func (t *TestFramework) requireMarkedBooked(transactionAliases ...string) {
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, transactionMetadataExists := t.Instance.Transaction(t.TransactionID(transactionAlias))

		require.True(t.test, transactionMetadataExists && transactionMetadata.IsBooked(), "transaction %s was not booked", transactionAlias)
	}
}
