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

func (e *EpochKVStore) RestoreLastPrunedEpoch() error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	var lowestEpoch *iotago.EpochIndex
	if err := e.streamEpochs(func(epoch iotago.EpochIndex) error {
		if lowestEpoch == nil || epoch < *lowestEpoch {
			lowestEpoch = &epoch
		}

		return nil
	}); err != nil {
		return err
	}

	if lowestEpoch != nil && *lowestEpoch > 0 {
		// We need to subtract 1 as lowestEpoch is the last epoch we have stored. Thus, the last pruned is -1.
		e.lastPrunedEpoch.MarkEvicted(*lowestEpoch - 1)
	}

	return nil
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

func (e *EpochKVStore) streamEpochs(consumer func(epoch iotago.EpochIndex) error) error {
	var innerErr error
	if storageErr := e.kv.IterateKeys(kvstore.EmptyPrefix, func(epochBytes []byte) (advance bool) {
		epoch, _, err := iotago.EpochIndexFromBytes(epochBytes)
		if err != nil {
			innerErr = ierrors.Wrapf(err, "failed to parse epoch index from bytes %v", epochBytes)
			return false
		}

		innerErr = consumer(epoch)

		return innerErr == nil
	}); storageErr != nil {
		return ierrors.Wrap(storageErr, "failed to iterate over epochs")
	}

	if innerErr != nil {
		return ierrors.Wrap(innerErr, "failed to stream epochs")
	}

	return nil
}

func (e *EpochKVStore) Prune(epoch iotago.EpochIndex, defaultPruningDelay iotago.EpochIndex) error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	// The epoch we're trying to prune already takes into account the defaultPruningDelay.
	// Therefore, we don't need to do anything if it is greater equal e.pruningDelay and take the difference otherwise.
	var pruningDelay iotago.EpochIndex
	if defaultPruningDelay >= e.pruningDelay {
		pruningDelay = 0
	} else {
		pruningDelay = e.pruningDelay - defaultPruningDelay
	}

	// No need to prune.
	if epoch < lo.Return1(e.lastPrunedEpoch.Index())+pruningDelay {
		return nil
	}

	target := epoch - pruningDelay
	err := e.kv.DeletePrefix(target.MustBytes())
	if err != nil {
		return ierrors.Wrapf(err, "failed to prune epoch store for realm %v at epoch %d", e.realm, target)
	}

	e.lastPrunedEpoch.MarkEvicted(target)

	return nil
}
