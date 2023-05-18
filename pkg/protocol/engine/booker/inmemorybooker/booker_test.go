package inmemorybooker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestBooker(t *testing.T) {
	workers := workerpool.NewGroup(t.Name())
	prunableStorage := prunable.New(database.Config{
		Engine:    hivedb.EngineMapDB,
		Directory: t.TempDir(),
	})
	committee := account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()).SelectAccounts()
	booker := New(workers, committee, blocks.New(eviction.NewState(prunableStorage.RootBlocks, eviction.WithRootBlocksEvictionDelay(3)), func() *iotago.SlotTimeProvider {
		return iotago.NewSlotTimeProvider(0, 10)
	}))

	booker.ledger = utxoledger.New(workers, mapdb.NewMapDB(), func() iotago.API {
		return iotago.LatestAPI(&iotago.ProtocolParameters{})
	}, committee, func(err error) {
		require.FailNow(t, err.Error(), "failed with error")
	})

	booker.conflictDAG = booker.ledger.ConflictDAG()

	//booker.Queue()

}
