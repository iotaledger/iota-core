package model

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

type PruningIndex struct {
	kv  kvstore.KVStore
	key kvstore.Realm

	memLastPrunedEpoch *EvictionIndex[iotago.EpochIndex]
	lastPrunedMutex    syncutils.RWMutex
}

func NewPruningIndex(kv kvstore.KVStore, key kvstore.Realm) *PruningIndex {
	return &PruningIndex{
		kv:                 kv,
		key:                key,
		memLastPrunedEpoch: NewEvictionIndex[iotago.EpochIndex](),
	}
}

func (e *PruningIndex) MarkEvicted(epoch iotago.EpochIndex) error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	e.memLastPrunedEpoch.MarkEvicted(epoch)

	return e.kv.Set(e.key, epoch.MustBytes())
}

func (e *PruningIndex) Index() (currentEpoch iotago.EpochIndex, valid bool) {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	return e.memLastPrunedEpoch.Index()
}

func (e *PruningIndex) NextIndex() iotago.EpochIndex {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	return e.memLastPrunedEpoch.NextIndex()
}

func (e *PruningIndex) RestoreFromDisk() error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	lastPrunedBytes, err := e.kv.Get(e.key)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}

		return err
	}

	lastPrunedEpoch, _, err := iotago.EpochIndexFromBytes(lastPrunedBytes)
	if err != nil {
		return err
	}

	e.memLastPrunedEpoch.MarkEvicted(lastPrunedEpoch)

	return nil
}
