package eviction

import (
	"testing"

	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestState_RootBlocks(t *testing.T) {
	prunableStorage := prunable.New(database.Config{
		Engine:    hivedb.EngineMapDB,
		Directory: t.TempDir(),
	})

	ts := NewTestFramework(t, prunableStorage, NewState(prunableStorage.RootBlocks, 0, WithRootBlocksEvictionDelay(3)))
	ts.CreateAndAddRootBlock("Genesis", 0, iotago.NewEmptyCommitment().MustID())
	ts.RequireActiveRootBlocks("Genesis")
	ts.RequireLastEvictedSlot(0, false)

	ts.CreateAndAddRootBlock("Root1.0", 1, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root1.1", 1, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root2.0", 2, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root3.0", 3, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root4.0", 4, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root4.1", 4, iotago.NewEmptyCommitment().MustID())
	ts.CreateAndAddRootBlock("Root5.0", 5, iotago.NewEmptyCommitment().MustID())

	ts.RequireActiveRootBlocks("Genesis")
	ts.RequireLastEvictedSlot(0, false)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")

	ts.Instance.EvictUntil(0)
	ts.RequireActiveRootBlocks("Genesis")
	ts.RequireLastEvictedSlot(0, true)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")

	ts.Instance.EvictUntil(1)
	ts.RequireActiveRootBlocks("Genesis", "Root1.0", "Root1.1")
	ts.RequireLastEvictedSlot(1, true)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")

	ts.Instance.EvictUntil(2)
	ts.RequireActiveRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0")
	ts.RequireLastEvictedSlot(2, true)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")

	ts.Instance.EvictUntil(3)
	// Genesis is evicted because the rootBlockEviction delay is 3: we keep the root blocks of the last 3 slots
	// starting from the last evicted one: 3, 2, 1.
	ts.RequireActiveRootBlocks("Root1.0", "Root1.1", "Root2.0", "Root3.0")
	ts.RequireLastEvictedSlot(3, true)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")

	ts.Instance.EvictUntil(4)
	ts.RequireActiveRootBlocks("Root2.0", "Root3.0", "Root4.0", "Root4.1")
	ts.RequireLastEvictedSlot(4, true)
	ts.RequireStorageRootBlocks("Genesis", "Root1.0", "Root1.1", "Root2.0", "Root3.0", "Root4.0", "Root4.1", "Root5.0")
}
