package txretainer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/sql"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunablesql"
	iotago "github.com/iotaledger/iota.go/v4"
)

type txRetainerTestsuite struct {
	T          *testing.T
	currentDB  *prunablesql.LockableGormDB
	TxRetainer *txretainer.TransactionRetainer

	latestCommitedSlot iotago.SlotIndex
	finalizedSlot      iotago.SlotIndex
}

func newTestSuite(t *testing.T) *txRetainerTestsuite {
	dbParams := sql.DatabaseParameters{
		Engine:   db.EngineSQLite,
		Path:     t.TempDir(),
		Filename: "test.db",
	}

	logger := log.NewLogger().NewChildLogger(t.Name())

	db, _, err := sql.New(logger.NewChildLogger("sql"), dbParams, true, []db.Engine{db.EngineSQLite})
	require.NoError(t, err)

	lockableDB := prunablesql.NewLockableGormDB(db)

	workers := workerpool.NewGroup("TxRetainer")
	defer workers.Shutdown()

	testSuite := &txRetainerTestsuite{
		T:         t,
		currentDB: lockableDB,
	}

	testSuite.TxRetainer = txretainer.New(workers, testSuite.currentDB.ExecDBFunc,
		func() iotago.SlotIndex {
			return testSuite.latestCommitedSlot
		}, func() iotago.SlotIndex {
			return testSuite.finalizedSlot
		}, func(err error) {
			require.NoError(t, err)
		},
		txretainer.WithDebugStoreErrorMessages(true),
	)

	require.NoError(t, testSuite.TxRetainer.CreateTables())
	require.NoError(t, testSuite.TxRetainer.AutoMigrate())

	return testSuite
}

func (ts *txRetainerTestsuite) Close() {
	_ = ts.currentDB.ExecDBFunc(func(database *gorm.DB) error {
		db, err := database.DB()
		require.NoError(ts.T, err)
		require.NoError(ts.T, db.Close())
		return nil
	})
}

func (ts *txRetainerTestsuite) SetCurrentDB(db *gorm.DB) {
	ts.currentDB = prunablesql.NewLockableGormDB(db)
}

func (ts *txRetainerTestsuite) SetLatestCommittedSlot(slot iotago.SlotIndex) {
	ts.latestCommitedSlot = slot
}

func (ts *txRetainerTestsuite) SetFinalizedSlot(slot iotago.SlotIndex) {
	ts.finalizedSlot = slot
}
