package eviction_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Testing         *testing.T
	Instance        *eviction.State
	Settings        *permanent.Settings
	prunableStorage *prunable.Prunable

	rootBlockIDs  *shrinkingmap.ShrinkingMap[string, iotago.BlockID]
	commitmentIDs *shrinkingmap.ShrinkingMap[iotago.BlockID, iotago.CommitmentID]

	idCounter uint64
	mutex     syncutils.RWMutex
}

func NewTestFramework(testing *testing.T, prunableStorage *prunable.Prunable, settings *permanent.Settings) *TestFramework {
	return &TestFramework{
		Testing:         testing,
		prunableStorage: prunableStorage,
		Instance:        eviction.NewState(settings, prunableStorage.RootBlocks),
		Settings:        settings,
		rootBlockIDs:    shrinkingmap.New[string, iotago.BlockID](),
		commitmentIDs:   shrinkingmap.New[iotago.BlockID, iotago.CommitmentID](),
	}
}

func (t *TestFramework) AdvanceActiveWindowToIndex(slot iotago.SlotIndex, isEmpty bool) {
	t.Instance.AdvanceActiveWindowToIndex(slot)
	if !isEmpty {
		require.NoError(t.Testing, t.Settings.AdvanceLatestNonEmptySlot(slot))
	}
}

func (t *TestFramework) CreateAndAddRootBlock(alias string, slot iotago.SlotIndex, commitmentID iotago.CommitmentID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	id := iotago.Identifier{}
	binary.LittleEndian.PutUint64(id[:], t.idCounter)
	blockID := iotago.NewBlockID(slot, id)
	blockID.RegisterAlias(alias)

	if !t.rootBlockIDs.Set(alias, blockID) {
		panic(fmt.Sprintf("alias %s already exists", alias))
	}
	t.commitmentIDs.Set(blockID, commitmentID)

	t.Instance.AddRootBlock(blockID, commitmentID)
}

func (t *TestFramework) RootBlock(alias string) (iotago.BlockID, iotago.CommitmentID) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	blockID, exists := t.rootBlockIDs.Get(alias)
	if !exists {
		t.Testing.Fatalf("expected alias %s to exist in rootBlockIDs", alias)
	}
	commitmentID, exists := t.commitmentIDs.Get(blockID)
	if !exists {
		t.Testing.Fatalf("expected blockID %s to exist in commitmentIDs", blockID)
	}

	return blockID, commitmentID
}

func (t *TestFramework) RootBlocks(aliases ...string) map[iotago.BlockID]iotago.CommitmentID {
	rootBlocks := make(map[iotago.BlockID]iotago.CommitmentID)
	for _, alias := range aliases {
		blockID, commitmentID := t.RootBlock(alias)
		rootBlocks[blockID] = commitmentID
	}

	return rootBlocks
}

func (t *TestFramework) RequireActiveRootBlocks(expected ...string) {
	expectedRootBlocks := t.RootBlocks(expected...)
	gotActiveRootBlocks := t.Instance.AllActiveRootBlocks()
	require.Equalf(t.Testing, expectedRootBlocks, gotActiveRootBlocks, "active root blocks do not match, expected: %s, got: %s", expectedRootBlocks, gotActiveRootBlocks)
}

func (t *TestFramework) RequireLastEvictedSlot(expectedSlot iotago.SlotIndex) {
	slot := t.Instance.LastEvictedSlot()
	require.Equal(t.Testing, expectedSlot, slot)
}

func (t *TestFramework) RequireStorageRootBlocks(expected ...string) {
	expectedRootBlocks := t.RootBlocks(expected...)

	for blockID, commitmentID := range expectedRootBlocks {
		rootBlockStorage, err := t.prunableStorage.RootBlocks(blockID.Slot())
		require.NoError(t.Testing, err)

		loadedCommitmentID, exists, err := rootBlockStorage.Load(blockID)
		require.NoError(t.Testing, err)
		require.True(t.Testing, exists, "expected blockID %s to exist in rootBlockStorage", blockID)
		require.Equal(t.Testing, commitmentID, loadedCommitmentID)
	}
}
