package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/retainer/blockretainer"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type BlockRetainerAction struct {
	Alias      string
	Action     action
	BlockState api.BlockState
}

type action int

const (
	none action = iota
	eventAccepted
	eventConfirmed
	eventDropped
)

type TestFramework struct {
	Instance *blockretainer.BlockRetainer
	stores   map[iotago.SlotIndex]*slotstore.BlockMetadataStore
	api      iotago.API
	test     *testing.T

	testBlocks   map[string]*blocks.Block
	testBlockIDs map[string]iotago.BlockID

	lastCommittedSlot iotago.SlotIndex
	lastFinalizedSlot iotago.SlotIndex
}

func NewTestFramework(t *testing.T) *TestFramework {
	t.Helper()

	tf := &TestFramework{
		stores:            make(map[iotago.SlotIndex]*slotstore.BlockMetadataStore),
		lastCommittedSlot: iotago.SlotIndex(0),
		testBlocks:        make(map[string]*blocks.Block),
		testBlockIDs:      make(map[string]iotago.BlockID),
		api: iotago.V3API(
			iotago.NewV3SnapshotProtocolParameters(
				iotago.WithTimeProviderOptions(0, time.Now().Unix(), 10, 3),
				iotago.WithLivenessOptions(5, 5, 1, 2, 3),
			),
		),
		test: t,
	}

	workers := workerpool.NewGroup(t.Name())

	storeFunc := func(slotIndex iotago.SlotIndex) (*slotstore.BlockMetadataStore, error) {
		if _, exists := tf.stores[slotIndex]; !exists {
			tf.stores[slotIndex] = slotstore.NewBlockMetadataStore(slotIndex, mapdb.NewMapDB())
		}

		return tf.stores[slotIndex], nil
	}

	lastFinalizedSlotFunc := func() iotago.SlotIndex {
		return tf.lastFinalizedSlot
	}

	errorHandlerFunc := func(err error) {
		require.NoError(t, err)
	}

	tf.Instance = blockretainer.New(module.NewTestModule(t), workers, storeFunc, lastFinalizedSlotFunc, errorHandlerFunc)

	return tf
}

func (tf *TestFramework) commitSlot(slot iotago.SlotIndex) {
	err := tf.Instance.CommitSlot(slot)
	require.NoError(tf.test, err)
}

func (tf *TestFramework) finalizeSlot(slot iotago.SlotIndex) {
	tf.lastFinalizedSlot = slot
}

func (tf *TestFramework) getBlock(alias string) *blocks.Block {
	if block, exists := tf.testBlocks[alias]; exists {
		return block
	}

	require.Errorf(tf.test, nil, "model block not found in the test framework")

	return nil
}

func (tf *TestFramework) getBlockID(alias string) iotago.BlockID {
	if blkID, exists := tf.testBlockIDs[alias]; exists {
		return blkID
	}

	require.Errorf(tf.test, nil, "model block not found in the test framework")

	return iotago.EmptyBlockID
}

func (tf *TestFramework) createBlock(alias string, slot iotago.SlotIndex) *blocks.Block {
	basicBlock := tpkg.RandBlock(tpkg.RandBasicBlockBody(tf.api, iotago.PayloadTaggedData), tf.api, 0)
	basicBlock.Header.IssuingTime = tf.api.TimeProvider().SlotStartTime(slot)
	modelBlock, err := model.BlockFromBlock(basicBlock)
	require.NoError(tf.test, err)

	block := blocks.NewBlock(modelBlock)

	tf.testBlocks[alias] = block
	tf.testBlockIDs[alias] = block.ID()

	return block
}

func (tf *TestFramework) initiateRetainerBlockFlow(currentSlot iotago.SlotIndex, aliases []string) {
	for _, alias := range aliases {
		err := tf.Instance.OnBlockBooked(tf.createBlock(alias, currentSlot))
		require.NoError(tf.test, err)
	}

	for _, alias := range aliases {
		blockID := tf.getBlockID(alias)
		expectedResp := &api.BlockMetadataResponse{
			BlockID:    blockID,
			BlockState: api.BlockStatePending,
		}

		res, err := tf.Instance.BlockMetadata(blockID)
		require.NoError(tf.test, err)
		require.Equal(tf.test, expectedResp, res, "block metadata mismatch for alias %s", alias)
	}
}

func (tf *TestFramework) triggerBlockRetainerAction(alias string, act action) error {
	var err error
	switch act {
	case none:
		// no action
	case eventAccepted:
		err = tf.Instance.OnBlockAccepted(tf.getBlock(alias))
	case eventConfirmed:
		err = tf.Instance.OnBlockConfirmed(tf.getBlock(alias))
	case eventDropped:
		err = tf.Instance.OnBlockDropped(tf.getBlock(alias))
	default:
		err = ierrors.Errorf("unknown action %d", act)
	}

	return err
}

func (tf *TestFramework) assertBlockMetadata(testActions []*BlockRetainerAction) {
	for _, act := range testActions {
		err := tf.triggerBlockRetainerAction(act.Alias, act.Action)
		require.NoError(tf.test, err)
	}

	for _, act := range testActions {
		blockID := tf.getBlockID(act.Alias)
		expectedResp := &api.BlockMetadataResponse{
			BlockID:    blockID,
			BlockState: act.BlockState,
		}
		res, err := tf.Instance.BlockMetadata(blockID)
		if act.BlockState == api.BlockStateUnknown {
			require.Error(tf.test, err)
			continue
		}
		require.NoError(tf.test, err)
		require.Equal(tf.test, expectedResp, res, "block metadata mismatch for alias %s", act.Alias)
	}
}

func (tf *TestFramework) assertError(act *BlockRetainerAction) {
	err := tf.triggerBlockRetainerAction(act.Alias, act.Action)
	require.Error(tf.test, err)
}
