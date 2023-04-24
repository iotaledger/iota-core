package mempooltests

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance mempool.MemPool

	stateByAlias                    map[string]ledger.State
	transactionsByAlias             map[string]mempool.Transaction
	transactionBookedTriggered      map[iotago.TransactionID]bool
	transactionBookedTriggeredMutex sync.RWMutex
	test                            *testing.T
}

func NewTestFramework(test *testing.T, instance mempool.MemPool) *TestFramework {
	t := &TestFramework{
		Instance:                   instance,
		stateByAlias:               make(map[string]ledger.State),
		transactionsByAlias:        make(map[string]mempool.Transaction),
		transactionBookedTriggered: make(map[iotago.TransactionID]bool),
		test:                       test,
	}

	t.setupHookedEvents()

	return t
}

func (t *TestFramework) setupHookedEvents() {
	t.Instance.Events().TransactionStored.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionStored with '%s'", metadata.ID())
		}
	})

	t.Instance.Events().TransactionSolid.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionSolid with '%s'", metadata.ID())
		}
	})

	t.Instance.Events().TransactionExecuted.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionExecuted with '%s'", metadata.ID())
		}
	})

	t.Instance.Events().TransactionBooked.Hook(func(metadata mempool.TransactionWithMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.Events.TransactionBooked with '%s'", metadata.ID())
		}

		require.True(t.test, metadata.IsBooked(), "transaction is not marked as booked")

		metadata.Outputs().Range(func(output mempool.StateWithMetadata) {
			t.stateByAlias[metadata.ID().Alias()+":"+strconv.Itoa(int(output.ID().Index()))] = output.State()
		})

		t.transactionBookedTriggeredMutex.Lock()
		defer t.transactionBookedTriggeredMutex.Unlock()

		t.transactionBookedTriggered[metadata.ID()] = true
	})
}

func (t *TestFramework) StateID(alias string) iotago.OutputID {
	if alias == "genesis" {
		return iotago.OutputID{}
	}

	state, exists := t.stateByAlias[alias]
	require.True(t.test, exists, "state with alias '%s' does not exist", alias)

	return state.ID()
}

func (t *TestFramework) TransactionID(alias string) iotago.TransactionID {
	transaction, exists := t.transactionsByAlias[alias]
	require.True(t.test, exists, "transaction with alias '%s' does not exist", alias)

	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to get transaction ID")

	return transactionID
}

func (t *TestFramework) StateReference(alias string) mempool.StateReference {
	return LedgerStateReference(t.StateID(alias))
}

func (t *TestFramework) ProcessTransaction(alias string, referencedStates []string, outputCount uint16) error {
	t.transactionsByAlias[alias] = NewTransaction(outputCount, lo.Map(referencedStates, t.StateReference)...)

	transactionID, err := t.transactionsByAlias[alias].ID()
	require.NoError(t.test, err, "failed to get transaction ID")
	transactionID.RegisterAlias(alias)

	return t.Instance.ProcessTransaction(t.transactionsByAlias[alias])
}

func (t *TestFramework) RequireBooked(transactionAliases ...string) {
	for _, transactionAlias := range transactionAliases {
		require.Eventuallyf(t.test, func() bool {
			transactionMetadata, exists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))

			return exists && transactionMetadata.IsBooked()
		}, 10*time.Second, 50*time.Millisecond, "transaction '%s' was not booked in time", transactionAlias)
	}

	t.transactionBookedTriggeredMutex.RLock()
	defer t.transactionBookedTriggeredMutex.RUnlock()

	for _, transactionAlias := range transactionAliases {
		require.True(t.test, t.transactionBookedTriggered[t.TransactionID(transactionAlias)], "transaction '%s' was not booked", transactionAlias)
	}
}
