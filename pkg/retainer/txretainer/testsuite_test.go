package txretainer_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	"github.com/iotaledger/iota-core/pkg/storage/clonablesql"
	iotago "github.com/iotaledger/iota.go/v4"
)

type txRetainerTestsuite struct {
	T          *testing.T
	database   *clonablesql.ClonableSQLiteDatabase
	TxRetainer *txretainer.TransactionRetainer

	latestCommittedSlot iotago.SlotIndex
	finalizedSlot       iotago.SlotIndex
}

func newClonableSQLiteDatabase(t *testing.T) *clonablesql.ClonableSQLiteDatabase {
	logger := log.NewLogger().NewChildLogger(t.Name())

	return clonablesql.NewClonableSQLiteDatabase(logger.NewChildLogger("tx-retainer-db"), t.TempDir(), "tx_retainer.db", func(err error) {
		require.NoError(t, err)
	})
}

func newTestSuite(t *testing.T) *txRetainerTestsuite {
	workers := workerpool.NewGroup("TxRetainer")
	defer workers.Shutdown()

	testSuite := &txRetainerTestsuite{
		T:        t,
		database: newClonableSQLiteDatabase(t),
	}

	testSuite.TxRetainer = txretainer.New(module.NewTestModule(t), workers, testSuite.database.ExecDBFunc(),
		func() iotago.SlotIndex {
			return testSuite.latestCommittedSlot
		}, func() iotago.SlotIndex {
			return testSuite.finalizedSlot
		}, func(err error) {
			require.NoError(t, err)
		},
		txretainer.WithDebugStoreErrorMessages(true),
	)

	return testSuite
}

func (ts *txRetainerTestsuite) Close() {
	ts.database.Shutdown()
	ts.TxRetainer.ShutdownEvent().Trigger()
}

func (ts *txRetainerTestsuite) SetLatestCommittedSlot(slot iotago.SlotIndex) {
	ts.latestCommittedSlot = slot
	if err := ts.TxRetainer.CommitSlot(slot); err != nil {
		panic(err)
	}
}

func (ts *txRetainerTestsuite) SetFinalizedSlot(slot iotago.SlotIndex) {
	ts.finalizedSlot = slot
}
