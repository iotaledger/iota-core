package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
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
	})

	return t
}

func (t *TestFramework) AddBlock(alias string) tipmanager.TipMetadata {
	return t.Instance.AddBlock(t.Block(alias))
}

func (t *TestFramework) CreateBlock(alias string, parents map[iotago.ParentsType][]string) *blocks.Block {
	blockBuilder := builder.NewBasicBlockBuilder(tpkg.TestAPI)
	blockBuilder.IssuingTime(time.Now())

	if strongParents, strongParentsExist := parents[iotago.StrongParentType]; strongParentsExist {
		blockBuilder.StrongParents(lo.Map(strongParents, t.BlockID))
	}
	if weakParents, weakParentsExist := parents[iotago.WeakParentType]; weakParentsExist {
		blockBuilder.WeakParents(lo.Map(weakParents, t.BlockID))
	}
	if shallowLikeParents, shallowLikeParentsExist := parents[iotago.ShallowLikeParentType]; shallowLikeParentsExist {
		blockBuilder.ShallowLikeParents(lo.Map(shallowLikeParents, t.BlockID))
	}

	block, err := blockBuilder.Build()
	require.NoError(t.test, err)

	modelBlock, err := model.BlockFromBlock(block)
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
		require.True(t.test, ds.NewSet(lo.Map(t.Instance.StrongTips(), tipmanager.TipMetadata.ID)...).Has(t.BlockID(alias)), "strongTips does not contain block '%s'", alias)
	}

	require.Equal(t.test, len(aliases), len(t.Instance.StrongTips()), "strongTips size does not match")
}
