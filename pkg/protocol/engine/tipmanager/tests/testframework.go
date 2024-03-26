//nolint:dupl
package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	tipmanagerv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/mock"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	Instance *tipmanagerv1.TipManager

	blockIDsByAlias    map[string]iotago.BlockID
	tipMetadataByAlias map[string]tipmanager.TipMetadata
	blocksByID         map[iotago.BlockID]*blocks.Block
	test               *testing.T
	time               time.Time

	manualPOA mock.ManualPOA

	API iotago.API
}

func NewTestFramework(t *testing.T) *TestFramework {
	t.Helper()

	tf := &TestFramework{
		blockIDsByAlias:    make(map[string]iotago.BlockID),
		tipMetadataByAlias: make(map[string]tipmanager.TipMetadata),
		blocksByID:         make(map[iotago.BlockID]*blocks.Block),
		test:               t,
		API:                tpkg.ZeroCostTestAPI,
		time:               time.Now(),
		manualPOA: *mock.NewManualPOA(iotago.SingleVersionProvider(tpkg.ZeroCostTestAPI),
			epochstore.NewStore[*account.SeatedAccounts](
				nil,
				mapdb.NewMapDB(),
				func(index iotago.EpochIndex) iotago.EpochIndex { return index },
				(*account.SeatedAccounts).Bytes,
				account.SeatedAccountsFromBytes),
		),
	}

	tf.blockIDsByAlias["Genesis"] = iotago.EmptyBlockID

	tf.Instance = tipmanagerv1.New(module.NewTestModule(t), func(blockID iotago.BlockID) (block *blocks.Block, exists bool) {
		block, exists = tf.blocksByID[blockID]
		return block, exists
	}, tf.manualPOA.CommitteeInSlot)

	return tf
}

func (t *TestFramework) Validator(alias string) iotago.AccountID {
	return t.manualPOA.AccountID(alias)
}

func (t *TestFramework) AddValidators(aliases ...string) {
	t.manualPOA.AddRandomAccounts(aliases...)

	for _, alias := range aliases {
		seat, exists := t.manualPOA.GetSeat(alias)
		if !exists {
			panic("seat does not exist")
		}

		t.Instance.AddSeat(seat)
	}
}

func (t *TestFramework) AddBlock(alias string) tipmanager.TipMetadata {
	t.tipMetadataByAlias[alias] = t.Instance.AddBlock(t.Block(alias))
	t.tipMetadataByAlias[alias].ID().RegisterAlias(alias)

	return t.tipMetadataByAlias[alias]
}

func (t *TestFramework) CreateBasicBlock(alias string, parents map[iotago.ParentsType][]string, optBlockBuilder ...func(*builder.BasicBlockBuilder)) *blocks.Block {
	blockBuilder := builder.NewBasicBlockBuilder(t.API)

	// Make sure that blocks don't have the same timestamp.
	t.time = t.time.Add(1)
	blockBuilder.IssuingTime(t.time)

	if strongParents, strongParentsExist := parents[iotago.StrongParentType]; strongParentsExist {
		blockBuilder.StrongParents(lo.Map(strongParents, t.BlockID))
	}
	if weakParents, weakParentsExist := parents[iotago.WeakParentType]; weakParentsExist {
		blockBuilder.WeakParents(lo.Map(weakParents, t.BlockID))
	}
	if shallowLikeParents, shallowLikeParentsExist := parents[iotago.ShallowLikeParentType]; shallowLikeParentsExist {
		blockBuilder.ShallowLikeParents(lo.Map(shallowLikeParents, t.BlockID))
	}

	if len(optBlockBuilder) > 0 {
		optBlockBuilder[0](blockBuilder)
	}

	block, err := blockBuilder.Build()
	require.NoError(t.test, err)

	modelBlock, err := model.BlockFromBlock(block)
	require.NoError(t.test, err)

	t.blocksByID[modelBlock.ID()] = blocks.NewBlock(modelBlock)
	t.blockIDsByAlias[alias] = modelBlock.ID()

	return t.blocksByID[modelBlock.ID()]
}

func (t *TestFramework) CreateValidationBlock(alias string, parents map[iotago.ParentsType][]string, optBlockBuilder ...func(blockBuilder *builder.ValidationBlockBuilder)) *blocks.Block {
	blockBuilder := builder.NewValidationBlockBuilder(t.API)

	// Make sure that blocks don't have the same timestamp.
	t.time = t.time.Add(1)
	blockBuilder.IssuingTime(t.time)

	if strongParents, strongParentsExist := parents[iotago.StrongParentType]; strongParentsExist {
		blockBuilder.StrongParents(lo.Map(strongParents, t.BlockID))
	}
	if weakParents, weakParentsExist := parents[iotago.WeakParentType]; weakParentsExist {
		blockBuilder.WeakParents(lo.Map(weakParents, t.BlockID))
	}
	if shallowLikeParents, shallowLikeParentsExist := parents[iotago.ShallowLikeParentType]; shallowLikeParentsExist {
		blockBuilder.ShallowLikeParents(lo.Map(shallowLikeParents, t.BlockID))
	}

	if len(optBlockBuilder) > 0 {
		optBlockBuilder[0](blockBuilder)
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

func (t *TestFramework) TipMetadata(alias string) tipmanager.TipMetadata {
	tipMetadata, tipMetadataExists := t.tipMetadataByAlias[alias]
	require.True(t.test, tipMetadataExists)

	return tipMetadata
}

func (t *TestFramework) BlockID(alias string) iotago.BlockID {
	blockID, blockIDExists := t.blockIDsByAlias[alias]
	require.True(t.test, blockIDExists, "blockID for alias '%s' does not exist", alias)

	return blockID
}

func (t *TestFramework) RequireStrongTips(aliases ...string) {
	for _, alias := range aliases {
		require.True(t.test, ds.NewSet(lo.Map(t.Instance.StrongTips(), tipmanager.TipMetadata.ID)...).Has(t.BlockID(alias)), "strongTips does not contain block '%s'", alias)
	}

	require.Equal(t.test, len(aliases), len(t.Instance.StrongTips()), "strongTips size does not match")
}

func (t *TestFramework) RequireValidationTips(aliases ...string) {
	for _, alias := range aliases {
		require.True(t.test, ds.NewSet(lo.Map(t.Instance.ValidationTips(), tipmanager.TipMetadata.ID)...).Has(t.BlockID(alias)), "validationTips does not contain block '%s'", alias)
	}

	require.Equal(t.test, len(aliases), len(t.Instance.ValidationTips()), "validationTips size does not match")
}

func (t *TestFramework) RequireLivenessThresholdReached(alias string, expected bool) {
	require.Equal(t.test, expected, t.TipMetadata(alias).LivenessThresholdReached().Get())
}
