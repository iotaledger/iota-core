package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

type EpochKVStore struct {
	realm        kvstore.Realm
	kv           kvstore.KVStore
	pruningDelay iotago.EpochIndex

	lastPrunedEpoch *model.EvictionIndex[iotago.EpochIndex]
	lastPrunedMutex syncutils.RWMutex
}

func NewEpochKVStore(realm kvstore.Realm, kv kvstore.KVStore, pruningDelay iotago.EpochIndex) *EpochKVStore {
	kv = lo.PanicOnErr(kv.WithExtendedRealm(realm))

	return &EpochKVStore{
		realm:           realm,
		kv:              kv,
		pruningDelay:    pruningDelay,
		lastPrunedEpoch: model.NewEvictionIndex[iotago.EpochIndex](),
	}
}

func (e *EpochKVStore) isTooOld(epoch iotago.EpochIndex) bool {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	prunedEpoch, hasPruned := e.lastPrunedEpoch.Index()

	return hasPruned && epoch <= prunedEpoch
}

func (e *EpochKVStore) RestoreLastPrunedEpoch(epoch iotago.EpochIndex) {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	e.lastPrunedEpoch.MarkEvicted(epoch)
}

func (e *EpochKVStore) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	return e.lastPrunedEpoch.Index()
}

func (e *EpochKVStore) GetEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
	if e.isTooOld(epoch) {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "epoch %d is too old", epoch)
	}

	return lo.PanicOnErr(e.kv.WithExtendedRealm(epoch.MustBytes())), nil
}

func (e *EpochKVStore) PruneUntilEpoch(epoch iotago.EpochIndex) error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	minStartEpoch := e.lastPrunedEpoch.NextIndex() + e.pruningDelay
	// No need to prune.
	if epoch < minStartEpoch {
		return nil
	}

	for currentIndex := minStartEpoch; currentIndex <= epoch; currentIndex++ {
		if err := e.prune(currentIndex); err != nil {
			return err
		}
	}

	return nil
}

func (e *EpochKVStore) prune(epoch iotago.EpochIndex) error {
	targetIndex := epoch - e.pruningDelay
	err := e.kv.DeletePrefix(targetIndex.MustBytes())
	if err != nil {
		return ierrors.Wrapf(err, "failed to prune epoch store for realm %v", e.realm)
	}

	e.lastPrunedEpoch.MarkEvicted(targetIndex)

	return nil
}
