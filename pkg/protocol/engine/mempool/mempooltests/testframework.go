package mempooltests

import (
	"testing"

	"github.com/stretchr/testify/require"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/vm"

	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance mempool.MemPool

	stateByAlias        map[string]vm.State
	transactionsByAlias map[string]mempool.Transaction
	test                *testing.T
}

func NewTestFramework(test *testing.T, instance mempool.MemPool) *TestFramework {
	return &TestFramework{
		Instance:            instance,
		stateByAlias:        make(map[string]vm.State),
		transactionsByAlias: make(map[string]mempool.Transaction),
		test:                test,
	}
}

func (t *TestFramework) StateID(alias string) iotago.OutputID {
	if alias == "genesis" {
		return iotago.OutputID{}
	}

	state, exists := t.stateByAlias[alias]
	require.True(t.test, exists, "state with alias '%s' does not exist", alias)

	return state.ID()
}

func (t *TestFramework) StateReference(alias string) mempool.StateReference {
	return LedgerStateReference(t.StateID(alias))
}

func (t *TestFramework) ProcessTransaction(alias string, referencedStates []string, outputCount uint16) error {
	t.transactionsByAlias[alias] = NewTransaction(outputCount, lo.Map(referencedStates, t.StateReference)...)

	return t.Instance.ProcessTransaction(t.transactionsByAlias[alias])
}
