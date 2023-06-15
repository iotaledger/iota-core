package blockgadget_test

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget/thresholdblockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type TestFramework struct {
	*testing.T
	api                iotago.API
	protocolParameters *iotago.ProtocolParameters

	blocks     *shrinkingmap.ShrinkingMap[string, *blocks.Block]
	blockCache *blocks.Blocks

	*sybilprotection.MockSybilProtection
	Instance blockgadget.Gadget
}

func NewTestFramework(test *testing.T) *TestFramework {
	t := &TestFramework{
		T:      test,
		blocks: shrinkingmap.New[string, *blocks.Block](),

		protocolParameters: &iotago.ProtocolParameters{
			Version:     3,
			NetworkName: test.Name(),
			Bech32HRP:   "rms",
			MinPoWScore: 0,
			RentStructure: iotago.RentStructure{
				VByteCost:    100,
				VBFactorData: 1,
				VBFactorKey:  10,
			},
			TokenSupply:           1_000_0000,
			GenesisUnixTimestamp:  uint32(time.Now().Truncate(10 * time.Second).Unix()),
			SlotDurationInSeconds: 10,
		},

		MockSybilProtection: sybilprotection.NewMockSybilProtection(),
	}

	t.api = iotago.LatestAPI(t.protocolParameters)

	evictionState := eviction.NewState(func(index iotago.SlotIndex) *prunable.RootBlocks {
		return prunable.NewRootBlocks(index, mapdb.NewMapDB())
	})

	t.blockCache = blocks.New(evictionState, t.api.SlotTimeProvider)
	t.Instance = thresholdblockgadget.New(t.blockCache, t.MockSybilProtection)

	genesisBlock := blocks.NewRootBlock(iotago.EmptyBlockID(), iotago.NewEmptyCommitment().MustID(), time.Unix(int64(t.protocolParameters.GenesisUnixTimestamp), 0))
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

	block, err := builder.NewBlockBuilder().
		StrongParents(t.BlockIDs(parents...)).
		Sign(t.MockSybilProtection.AccountID(issuerAlias), priv).
		Build()
	require.NoError(t, err)

	modelBlock, err := model.BlockFromBlock(block, t.api)
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

func (t *TestFramework) TrackWitnessWeight(alias string, issuerAlias string, parents ...string) {
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
