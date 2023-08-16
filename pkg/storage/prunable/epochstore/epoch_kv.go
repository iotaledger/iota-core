package epochstore

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
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

	return epoch < lo.Return1(e.lastPrunedEpoch.Index())
}

func (e *EpochKVStore) GetEpoch(epoch iotago.EpochIndex) kvstore.KVStore {
	if e.isTooOld(epoch) {
		return nil
	}

	return lo.PanicOnErr(e.kv.WithExtendedRealm(epoch.MustBytes()))
}

func (e *EpochKVStore) PruneUntilEpoch(epoch iotago.EpochIndex) error {
	e.lastPrunedMutex.RLock()
	start := e.lastPrunedEpoch.NextIndex()
	e.lastPrunedMutex.RUnlock()

	for currentIndex := start; currentIndex <= epoch; currentIndex++ {
		if err := e.prune(currentIndex); err != nil {
			return err
		}
	}

	return nil
}

func (e *EpochKVStore) prune(epoch iotago.EpochIndex) error {
	if e.isTooOld(epoch) {
		return ierrors.Errorf("epoch %d is already pruned", epoch)
	}

	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	if epoch < e.pruningDelay {
		return ierrors.Errorf("epoch index %d is smaller than pruning delay %d", epoch, e.pruningDelay)
	}

	targetIndex := epoch - e.pruningDelay
	err := e.kv.DeletePrefix(targetIndex.MustBytes())
	if err != nil {
		return ierrors.Wrapf(err, "failed to prune epoch store for realm %v", e.realm)
	}

	e.lastPrunedEpoch.MarkEvicted(targetIndex)

	return nil
}
