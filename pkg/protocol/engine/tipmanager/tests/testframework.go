package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type TestFramework struct {
	Instance *tipmanagerv1.TipManager

	blockIDsByAlias map[string]iotago.BlockID
	blocksByID      map[iotago.BlockID]*blocks.Block
	test            *testing.T
}

func NewTestFramework(test *testing.T) *TestFramework {
	t := &TestFramework{
		blockIDsByAlias: make(map[string]iotago.BlockID),
		blocksByID:      make(map[iotago.BlockID]*blocks.Block),
		test:            test,
	}

	t.blockIDsByAlias["Genesis"] = iotago.EmptyBlockID()

	t.Instance = tipmanagerv1.NewTipManager(func(blockID iotago.BlockID) (block *blocks.Block, exists bool) {
		block, exists = t.blocksByID[blockID]
		return block, exists
	}, func() iotago.BlockIDs {
		return iotago.BlockIDs{iotago.EmptyBlockID()}
	})
	t.Instance.SetConflictDAG(conflictdagv1.New[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower](account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()).SelectAccounts().SeatCount))

	return t
}

func (t *TestFramework) AddBlock(alias string) {
	t.Instance.AddBlock(t.Block(alias))
}

func (t *TestFramework) CreateBlock(alias string, parents map[model.ParentsType][]string) *blocks.Block {
	blockBuilder := builder.NewBlockBuilder()
	blockBuilder.IssuingTime(time.Now())

	if strongParents, strongParentsExist := parents[model.StrongParentType]; strongParentsExist {
		blockBuilder.StrongParents(lo.Map(strongParents, t.BlockID))
	}
	if weakParents, weakParentsExist := parents[model.WeakParentType]; weakParentsExist {
		blockBuilder.WeakParents(lo.Map(weakParents, t.BlockID))
	}
	if shallowLikeParents, shallowLikeParentsExist := parents[model.ShallowLikeParentType]; shallowLikeParentsExist {
		blockBuilder.ShallowLikeParents(lo.Map(shallowLikeParents, t.BlockID))
	}

	block, err := blockBuilder.Build()
	require.NoError(t.test, err)

	modelBlock, err := model.BlockFromBlock(block, iotago.V3API(&protoParams))
	require.NoError(t.test, err)

	t.blocksByID[modelBlock.ID()] = blocks.NewBlock(modelBlock)
	t.blockIDsByAlias[alias] = modelBlock.ID()

	return t.blocksByID[modelBlock.ID()]
}

func (t *TestFramework) Block(alias string) *blocks.Block {
	blockID, blockIDExists := t.blockIDsByAlias[alias]
	require.True(t.test, blockIDExists)

	block, blockExists := t.blocksByID[blockID]
	require.True(t.test, blockExists)

	return block
}

func (t *TestFramework) BlockID(alias string) iotago.BlockID {
	blockID, blockIDExists := t.blockIDsByAlias[alias]
	require.True(t.test, blockIDExists, "blockID for alias '%s' does not exist", alias)

	return blockID
}

func (t *TestFramework) AssertStrongTips(aliases ...string) {
	for _, alias := range aliases {
		require.True(t.test, advancedset.New(lo.Map(t.Instance.StrongTipSet(), (*blocks.Block).ID)...).Has(t.BlockID(alias)), "strongTips does not contain block '%s'", alias)
	}

	require.Equal(t.test, len(aliases), len(t.Instance.StrongTipSet()), "strongTips size does not match")
}

var protoParams = iotago.ProtocolParameters{
	Version:     3,
	NetworkName: "test",
	Bech32HRP:   "rms",
	MinPoWScore: 0,
	RentStructure: iotago.RentStructure{
		VByteCost:    100,
		VBFactorKey:  10,
		VBFactorData: 1,
	},
	TokenSupply:           5000,
	GenesisUnixTimestamp:  uint32(time.Now().Unix()),
	SlotDurationInSeconds: 10,
}
