package model

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
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

func (e *PruningIndex) ShouldPrune(newIndex iotago.EpochIndex) bool {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	prunedEpoch, _ := e.memLastPrunedEpoch.Index()

	return newIndex > prunedEpoch
}

func (e *PruningIndex) MarkEvicted(index iotago.EpochIndex) error {
	e.lastPrunedMutex.Lock()
	defer e.lastPrunedMutex.Unlock()

	e.memLastPrunedEpoch.MarkEvicted(index)

	return e.kv.Set(e.key, lo.Return1(index.Bytes()))
}

func (e *PruningIndex) Index() (current iotago.EpochIndex, valid bool) {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	return e.memLastPrunedEpoch.Index()
}

func (e *PruningIndex) NextIndex() iotago.EpochIndex {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	return e.memLastPrunedEpoch.NextIndex()
}

func (e *PruningIndex) IsEvicted(index iotago.EpochIndex) bool {
	e.lastPrunedMutex.RLock()
	defer e.lastPrunedMutex.RUnlock()

	lastPruned, _ := e.memLastPrunedEpoch.Index()

	return index <= lastPruned
}

func (e *PruningIndex) RestoreFromDisk() error {
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
