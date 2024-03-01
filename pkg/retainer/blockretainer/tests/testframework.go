package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore/mapdb"
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
	eventBooked
	eventAccepted
	eventConfirmed
	eventDropped
)

// todo check if event was triggered

type TestFramework struct {
	Instance *blockretainer.BlockRetainer
	stores   map[iotago.SlotIndex]*slotstore.BlockMetadataStore
	api      iotago.API
	test     *testing.T

	testBlockIDs map[string]iotago.BlockID

	lastCommittedSlot iotago.SlotIndex
	lastFinalizedSlot iotago.SlotIndex
}

func NewTestFramework(test *testing.T) *TestFramework {
	tf := &TestFramework{
		stores:            make(map[iotago.SlotIndex]*slotstore.BlockMetadataStore),
		lastCommittedSlot: iotago.SlotIndex(0),
		testBlockIDs:      make(map[string]iotago.BlockID),
		api: iotago.V3API(
			iotago.NewV3SnapshotProtocolParameters(
				iotago.WithTimeProviderOptions(0, time.Now().Unix(), 10, 3),
				iotago.WithLivenessOptions(5, 5, 1, 2, 3),
			),
		),
		test: test,
	}

	workers := workerpool.NewGroup(test.Name())

	storeFunc := func(slotIndex iotago.SlotIndex) (*slotstore.BlockMetadataStore, error) {
		if _, exists := tf.stores[slotIndex]; !exists {
			tf.stores[slotIndex] = slotstore.NewBlockMetadataStore(slotIndex, mapdb.NewMapDB())
		}

		return tf.stores[slotIndex], nil
	}

	latestCommittedSlotFunc := func() iotago.SlotIndex {
		return tf.lastCommittedSlot
	}

	lastFinalizedSlotFunc := func() iotago.SlotIndex {
		return tf.lastFinalizedSlot
	}

	errorHandlerFunc := func(err error) {
		require.NoError(test, err)
	}

	tf.Instance = blockretainer.New(workers, storeFunc, latestCommittedSlotFunc, lastFinalizedSlotFunc, errorHandlerFunc)

	return tf
}

func (tf *TestFramework) commitSlot(slot iotago.SlotIndex) {
	tf.lastCommittedSlot = slot
}

func (tf *TestFramework) finalizeSlot(slot iotago.SlotIndex) {
	tf.lastFinalizedSlot = slot
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

		// todo check if event was triggered
	}
}

func (tf *TestFramework) assertBlockMetadata(testActions []*BlockRetainerAction) {
	for _, act := range testActions {
		switch act.Action {
		case none:
			// no action
		case eventAccepted:
			err := tf.Instance.OnBlockAccepted(tf.getBlockID(act.Alias))
			require.NoError(tf.test, err)
		case eventConfirmed:
			err := tf.Instance.OnBlockConfirmed(tf.getBlockID(act.Alias))
			require.NoError(tf.test, err)
		case eventDropped:
			err := tf.Instance.OnBlockDropped(tf.getBlockID(act.Alias))
			require.NoError(tf.test, err)
		default:
			require.Errorf(tf.test, nil, "unknown action")
		}
	}

	for _, act := range testActions {
		blockID := tf.getBlockID(act.Alias)
		expectedResp := &api.BlockMetadataResponse{
			BlockID:    blockID,
			BlockState: act.BlockState,
		}
		res, err := tf.Instance.BlockMetadata(blockID)
		require.NoError(tf.test, err)
		require.Equal(tf.test, expectedResp, res, "block metadata mismatch for alias %s", act.Alias)
	}
}
