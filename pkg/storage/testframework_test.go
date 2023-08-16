package storage_test

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

const (
	B  int64 = 1
	KB       = 1024 * B
	MB       = 1024 * KB
	GB       = 1024 * MB

	dataSize = 8192
)

type TestFramework struct {
	t           *testing.T
	Instance    *storage.Storage
	apiProvider api.Provider
}

func NewTestFramework(t *testing.T, storageOpts ...options.Option[storage.Storage]) *TestFramework {
	errorHandler := func(err error) {
		t.Error(err)
	}

	instance := storage.New(t.TempDir(), 0, errorHandler, storageOpts...)
	require.NoError(t, instance.Settings().StoreProtocolParametersForStartEpoch(iotago.NewV3ProtocolParameters(), 0))

	return &TestFramework{
		t:           t,
		Instance:    instance,
		apiProvider: instance.Settings().APIProvider(),
	}
}

func (f *TestFramework) Shutdown() {
	f.Instance.Shutdown()
}

func (f *TestFramework) GeneratePrunableData(epoch iotago.EpochIndex, size int64) {
	initialStorageSize := f.Instance.PrunableDatabaseSize()

	apiForEpoch := f.apiProvider.APIForEpoch(epoch)
	endSlot := apiForEpoch.TimeProvider().EpochEnd(epoch)

	var createdBytes int64
	for createdBytes < size {
		block := tpkg.RandProtocolBlock(&iotago.BasicBlock{
			StrongParents: tpkg.SortedRandBlockIDs(1 + rand.Intn(iotago.BlockMaxParents)),
			Payload:       &iotago.TaggedData{Data: make([]byte, 8192)},
			BurnedMana:    1000,
		}, apiForEpoch)

		modelBlock, err := model.BlockFromBlock(block, apiForEpoch)
		require.NoError(f.t, err)

		err = f.Instance.Prunable.Blocks(endSlot).Store(modelBlock)
		require.NoError(f.t, err)

		createdBytes += int64(len(modelBlock.Data()))
		createdBytes += iotago.SlotIdentifierLength
	}

	f.AssertPrunableSizeGreater(initialStorageSize + size)

	fmt.Printf("> created %d MB of bucket prunable data\n\tPermanent: %dMB\n\tPrunable: %dMB\n", createdBytes/MB, f.Instance.PermanentDatabaseSize()/MB, f.Instance.PrunableDatabaseSize()/MB)
}

func (f *TestFramework) GenerateSemiPermanentData(epoch iotago.EpochIndex, size int64) {
	// TODO:
	//  3. generate data of some size in in semi-permanent storage: for all individual storages

}

func (f *TestFramework) GeneratePermanentData(size int64) {
	initialStorageSize := f.Instance.PermanentDatabaseSize()

	// Use as dummy to generate some data.
	kv := f.Instance.Permanent.Ledger()

	var count uint64
	var createdBytes int64
	for createdBytes < size {

		key := make([]byte, 8)
		binary.LittleEndian.PutUint64(key, count)
		err := kv.Set(key, tpkg.RandBytes(dataSize))
		require.NoError(f.t, err)

		createdBytes += int64(dataSize) + 8 // for key
		count++
	}

	require.NoError(f.t, kv.Flush())

	f.AssertPermanentSizeGreater(initialStorageSize + size)
	fmt.Printf("> created %d MB of permanent data\n\tPermanent: %dMB\n\tPrunable: %dMB\n", createdBytes/MB, f.Instance.PermanentDatabaseSize()/MB, f.Instance.PrunableDatabaseSize()/MB)
}

func (f *TestFramework) AssertStorageSizeGreater(expected int64) {
	require.GreaterOrEqual(f.t, f.Instance.Size(), expected)
}

func (f *TestFramework) AssertPrunableSizeGreater(expected int64) {
	require.GreaterOrEqual(f.t, f.Instance.PrunableDatabaseSize(), expected)
}

func (f *TestFramework) AssertPermanentSizeGreater(expected int64) {
	require.GreaterOrEqual(f.t, f.Instance.PermanentDatabaseSize(), expected)
}

func (f *TestFramework) AssertStorageSizeBelow(expected int64) {
	require.LessOrEqual(f.t, f.Instance.Size(), expected)
}

func (f *TestFramework) AssertPrunedUntil(expectedPrunedUntil iotago.EpochIndex, expectedHasPruned bool) {
	lastPruned, hasPruned := f.Instance.LastPrunedEpoch()
	require.Equal(f.t, expectedHasPruned, hasPruned)
	require.Equal(f.t, expectedPrunedUntil, lastPruned)

	// TODO: make sure that all the epochs until this point are actually pruned and files deleted
	//   -> for semi permanent storage we need to make sure that correct pruning delays are adhered to, should probably be specified (in the test) and tested accordingly
}
