package blockgadget_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/mock"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	*testing.T

	blocks     *shrinkingmap.ShrinkingMap[string, *blocks.Block]
	blockCache *blocks.Blocks

	SeatManager *mock.ManualPOA
	Instance    blockgadget.Gadget
	Events      *blockgadget.Events
}

func NewTestFramework(test *testing.T) *TestFramework {
	t := &TestFramework{
		T:      test,
		blocks: shrinkingmap.New[string, *blocks.Block](),

		SeatManager: mock.NewManualPOA(iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI), epochstore.NewStore(kvstore.Realm{}, mapdb.NewMapDB(), func(_ iotago.EpochIndex) iotago.EpochIndex { return 0 }, (*account.SeatedAccounts).Bytes, account.SeatedAccountsFromBytes)),
	}

	prunableStorage := prunable.New(database.Config{
		Engine:    db.EngineMapDB,
		Directory: t.TempDir(),
	}, iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI), func(err error) { panic(err) })

	newSettings := permanent.NewSettings(mapdb.NewMapDB())
	newSettings.StoreProtocolParametersForStartEpoch(tpkg.ZeroCostTestAPI.ProtocolParameters(), 0)

	evictionState := eviction.NewState(newSettings, prunableStorage.RootBlocks)

	t.blockCache = blocks.New(evictionState, iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI))
	instance := thresholdblockgadget.New(module.NewTestModule(test), t.blockCache, t.SeatManager, func(err error) {
		fmt.Printf(">> Gadget.Error: %s\n", err)
	})

	t.Events = instance.Events()
	t.Instance = instance

	genesisBlock := blocks.NewRootBlock(tpkg.ZeroCostTestAPI.ProtocolParameters().GenesisBlockID(), iotago.NewEmptyCommitment(tpkg.ZeroCostTestAPI).MustID(), time.Unix(tpkg.ZeroCostTestAPI.TimeProvider().GenesisUnixTime(), 0))
	t.blocks.Set("Genesis", genesisBlock)
	genesisBlock.ID().RegisterAlias("Genesis")
	evictionState.AddRootBlock(genesisBlock.ID(), genesisBlock.SlotCommitmentID())

	return t
}

func (t *TestFramework) Block(alias string) *blocks.Block {
	block, exist := t.blocks.Get(alias)
	if !exist {
		panic(fmt.Sprintf("block %s not registered", alias))
	}

	return block
}

func (t *TestFramework) BlockID(alias string) iotago.BlockID {
	return t.Block(alias).ID()
}

func (t *TestFramework) BlockIDs(aliases ...string) []iotago.BlockID {
	return lo.Map(aliases, func(alias string) iotago.BlockID {
		return t.BlockID(alias)
	})
}

func (t *TestFramework) Blocks(aliases ...string) []*blocks.Block {
	return lo.Map(aliases, func(alias string) *blocks.Block {
		return t.Block(alias)
	})
}

func (t *TestFramework) CreateBlock(alias string, issuerAlias string, parents ...string) *blocks.Block {
	if len(parents) == 0 {
		panic("no parents provided")
	}

	// We don't care about the actual signature here.
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	block, err := builder.NewValidationBlockBuilder(tpkg.ZeroCostTestAPI).
		StrongParents(t.BlockIDs(parents...)).
		Sign(t.SeatManager.AccountID(issuerAlias), priv).
		IssuingTime(time.Now()).
		Build()
	require.NoError(t, err)

	modelBlock, err := model.BlockFromBlock(block)
	require.NoError(t, err)

	blocksBlock := blocks.NewBlock(modelBlock)
	t.registerBlock(alias, blocksBlock)

	return blocksBlock
}

func (t *TestFramework) registerBlock(alias string, block *blocks.Block) {
	t.blocks.Set(alias, block)
	block.ID().RegisterAlias(alias)
	require.True(t.T, t.blockCache.StoreBlock(block))

	block.SetBooked()
}

func (t *TestFramework) CreateBlockAndTrackWitnessWeight(alias string, issuerAlias string, parents ...string) {
	t.Instance.TrackWitnessWeight(t.CreateBlock(alias, issuerAlias, parents...))
}

func (t *TestFramework) assertBlocksInCacheWithFunc(expectedBlocks []*blocks.Block, expectedPropertyState bool, propertyFunc func(*blocks.Block) bool, propertyFuncDescription string) {
	for _, block := range expectedBlocks {
		actual := propertyFunc(block)
		require.Equalf(t.T, expectedPropertyState, actual, "%s: %s should be %v, got %v", propertyFuncDescription, block.ID(), expectedPropertyState, actual)
	}
}

func (t *TestFramework) AssertBlocksPreAccepted(expectedBlocks []*blocks.Block, expectedPreAccepted bool) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedPreAccepted, (*blocks.Block).IsPreAccepted, "AssertBlocksPreAccepted")
}

func (t *TestFramework) AssertBlocksAccepted(expectedBlocks []*blocks.Block, expectedAccepted bool) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedAccepted, (*blocks.Block).IsAccepted, "AssertBlocksAccepted")
}

func (t *TestFramework) AssertBlocksPreConfirmed(expectedBlocks []*blocks.Block, expectedPreConfirmed bool) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedPreConfirmed, (*blocks.Block).IsPreConfirmed, "AssertBlocksPreConfirmed")
}

func (t *TestFramework) AssertBlocksConfirmed(expectedBlocks []*blocks.Block, expectedConfirmed bool) {
	t.assertBlocksInCacheWithFunc(expectedBlocks, expectedConfirmed, (*blocks.Block).IsConfirmed, "AssertBlocksConfirmed")
}
