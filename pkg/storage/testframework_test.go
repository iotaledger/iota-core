package storage_test

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/account"
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
)

type TestFramework struct {
	t           *testing.T
	Instance    *storage.Storage
	apiProvider api.Provider

	uniqueKeyCounter uint64
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

func (t *TestFramework) Shutdown() {
	t.Instance.Shutdown()
}

func (t *TestFramework) GeneratePrunableData(epoch iotago.EpochIndex, size int64) {
	initialStorageSize := t.Instance.PrunableDatabaseSize()

	apiForEpoch := t.apiProvider.APIForEpoch(epoch)
	endSlot := apiForEpoch.TimeProvider().EpochEnd(epoch)

	var createdBytes int64
	for createdBytes < size {
		block := tpkg.RandProtocolBlock(&iotago.BasicBlock{
			StrongParents: tpkg.SortedRandBlockIDs(1 + rand.Intn(iotago.BlockMaxParents)),
			Payload:       &iotago.TaggedData{Data: make([]byte, 8192)},
			BurnedMana:    1000,
		}, apiForEpoch)

		modelBlock, err := model.BlockFromBlock(block, apiForEpoch)
		require.NoError(t.t, err)

		err = t.Instance.Prunable.Blocks(endSlot).Store(modelBlock)
		require.NoError(t.t, err)

		createdBytes += int64(len(modelBlock.Data()))
		createdBytes += iotago.SlotIdentifierLength
	}

	t.AssertPrunableSizeGreater(initialStorageSize + size)

	fmt.Printf("> created %d MB of bucket prunable data\n\tPermanent: %dMB\n\tPrunable: %dMB\n", createdBytes/MB, t.Instance.PermanentDatabaseSize()/MB, t.Instance.PrunableDatabaseSize()/MB)
}

func (t *TestFramework) GenerateSemiPermanentData(epoch iotago.EpochIndex) {
	rewardsKV := lo.PanicOnErr(t.Instance.Prunable.Rewards(epoch).WithExtendedRealm(uint64ToBytes(epoch)))
	poolStatsStore := t.Instance.Prunable.PoolStats()
	decidedUpgradeSignalsStore := t.Instance.Prunable.DecidedUpgradeSignals()
	committeeStore := t.Instance.Prunable.Committee()

	var err error
	var createdBytes int64

	for i := 0; i < 200; i++ {
		createdBytes += t.storeRandomData(rewardsKV, 32)
	}

	poolStatsModel := &model.PoolsStats{
		TotalStake:          1,
		TotalValidatorStake: 2,
		ProfitMargin:        3,
	}
	err = poolStatsStore.Store(epoch, poolStatsModel)
	require.NoError(t.t, err)
	createdBytes += int64(len(lo.PanicOnErr(poolStatsModel.Bytes()))) + 8 // for epoch key

	versionAndHash := model.VersionAndHash{
		Version: 1,
		Hash:    iotago.Identifier{2},
	}
	err = decidedUpgradeSignalsStore.Store(epoch, versionAndHash)
	require.NoError(t.t, err)
	createdBytes += int64(len(lo.PanicOnErr(versionAndHash.Bytes()))) + 8 // for epoch key

	accounts := account.NewAccounts()
	accounts.Set(tpkg.RandAccountID(), &account.Pool{})
	err = committeeStore.Store(epoch, accounts)
	require.NoError(t.t, err)
	createdBytes += int64(len(lo.PanicOnErr(accounts.Bytes()))) + 8 // for epoch key
}

func (t *TestFramework) GeneratePermanentData(size int64) {
	initialStorageSize := t.Instance.PermanentDatabaseSize()

	// Use as dummy to generate some data.
	kv := t.Instance.Permanent.Ledger()

	var createdBytes int64
	for createdBytes < size {
		createdBytes += t.storeRandomData(kv, 8192)
	}

	require.NoError(t.t, kv.Flush())

	t.AssertPermanentSizeGreater(initialStorageSize + size)
	fmt.Printf("> created %d MB of permanent data\n\tPermanent: %dMB\n\tPrunable: %dMB\n", createdBytes/MB, t.Instance.PermanentDatabaseSize()/MB, t.Instance.PrunableDatabaseSize()/MB)
}

func (t *TestFramework) storeRandomData(kv kvstore.KVStore, size int64) int64 {
	err := kv.Set(uint64ToBytes(t.uniqueKeyCounter), tpkg.RandBytes(int(size)))
	require.NoError(t.t, err)

	t.uniqueKeyCounter++

	return size + 8 // for key
}

func uint64ToBytes[V ~uint64](v V) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func (t *TestFramework) AssertStorageSizeGreater(expected int64) {
	require.GreaterOrEqual(t.t, t.Instance.Size(), expected)
}

func (t *TestFramework) AssertPrunableSizeGreater(expected int64) {
	require.GreaterOrEqual(t.t, t.Instance.PrunableDatabaseSize(), expected)
}

func (t *TestFramework) AssertPermanentSizeGreater(expected int64) {
	require.GreaterOrEqual(t.t, t.Instance.PermanentDatabaseSize(), expected)
}

func (t *TestFramework) AssertStorageSizeBelow(expected int64) {
	require.LessOrEqual(t.t, t.Instance.Size(), expected)
}

func (t *TestFramework) AssertPrunedUntil(expectedPrunedUntil iotago.EpochIndex, expectedHasPruned bool) {
	lastPruned, hasPruned := t.Instance.LastPrunedEpoch()
	require.Equal(t.t, expectedHasPruned, hasPruned)
	require.Equal(t.t, expectedPrunedUntil, lastPruned)

	// TODO: make sure that all the epochs until this point are actually pruned and files deleted
	//   -> for semi permanent storage we need to make sure that correct pruning delays are adhered to, should probably be specified (in the test) and tested accordingly
}
