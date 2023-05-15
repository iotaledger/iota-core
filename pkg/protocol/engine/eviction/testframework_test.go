package eviction_test

import (
	"encoding/binary"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Testing         *testing.T
	Instance        *eviction.State
	prunableStorage *prunable.Prunable

	rootBlockIDs  *shrinkingmap.ShrinkingMap[string, iotago.BlockID]
	commitmentIDs *shrinkingmap.ShrinkingMap[iotago.BlockID, iotago.CommitmentID]

	idCounter uint64
	mutex     sync.RWMutex
}

func NewTestFramework(testing *testing.T, prunableStorage *prunable.Prunable, instance *eviction.State) *TestFramework {
	return &TestFramework{
		Testing:         testing,
		prunableStorage: prunableStorage,
		Instance:        instance,
		rootBlockIDs:    shrinkingmap.New[string, iotago.BlockID](),
		commitmentIDs:   shrinkingmap.New[iotago.BlockID, iotago.CommitmentID](),
	}
}

func (t *TestFramework) CreateAndAddRootBlock(alias string, slot iotago.SlotIndex, commitmentID iotago.CommitmentID) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	id := iotago.Identifier{}
	binary.LittleEndian.PutUint64(id[:], t.idCounter)
	blockID := iotago.NewSlotIdentifier(slot, id)
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
	gotActiveRootBlocks := t.Instance.ActiveRootBlocks()
	require.Equalf(t.Testing, expectedRootBlocks, gotActiveRootBlocks, "active root blocks do not match, expected: %v, got: %v", expectedRootBlocks, gotActiveRootBlocks)
}

func (t *TestFramework) RequireLastEvictedSlot(expectedSlot iotago.SlotIndex) {
	slot := t.Instance.LastEvictedSlot()
	require.Equal(t.Testing, expectedSlot, slot)
}

func (t *TestFramework) RequireStorageRootBlocks(expected ...string) {
	expectedRootBlocks := t.RootBlocks(expected...)

	for blockID, commitmentID := range expectedRootBlocks {
		rootBlockStorage := t.prunableStorage.RootBlocks(blockID.Index())
		require.NotNil(t.Testing, rootBlockStorage)

		loadedBlockID, loadedCommitmentID, err := rootBlockStorage.Load(blockID)
		require.NoError(t.Testing, err)
		require.Equal(t.Testing, blockID, loadedBlockID)
		require.Equal(t.Testing, commitmentID, loadedCommitmentID)
	}
}
