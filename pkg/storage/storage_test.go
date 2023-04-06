package storage

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

func Test(t *testing.T) {
	storageDirectory := t.TempDir()

	slotTimeProvider := iotago.NewSlotTimeProvider(time.Now().Unix(), 10)
	iotaBlock, err := builder.NewBlockBuilder().StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).Build()
	require.NoError(t, err)
	emptyBlock, err := model.BlockFromBlock(iotaBlock, iotago.V3API(&iotago.ProtocolParameters{}), slotTimeProvider)
	require.NoError(t, err)

	storage := New(storageDirectory, 1)
	require.NoError(t, storage.Settings.SetLatestStateMutationSlot(10))
	genesisCommitment := iotago.NewEmptyCommitment()
	require.NoError(t, storage.Commitments.Store(genesisCommitment))
	require.NoError(t, storage.Commitments.Store(iotago.NewCommitment(1, genesisCommitment.MustID(), iotago.Identifier{}, 0)))
	require.NoError(t, storage.Blocks.Store(emptyBlock))
	fmt.Println(storage.Blocks.Load(emptyBlock.BlockID()))

	storage.databaseManager.Flush(0)

	storage.Shutdown()

	storage = New(storageDirectory, 1)
	fmt.Println(lo.PanicOnErr(storage.Commitments.Load(0)), lo.PanicOnErr(storage.Commitments.Load(1)))
	require.Equal(t, iotago.SlotIndex(10), storage.Settings.LatestStateMutationSlot())

	fmt.Println(storage.Blocks.Load(emptyBlock.BlockID()))

	storage.Shutdown()
}
