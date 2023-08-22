package model

import (
	"github.com/iotaledger/hive.go/kvstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type PruningIndex struct {
	kv  kvstore.KVStore
	key kvstore.Realm
}

func NewPruningIndex(kv kvstore.KVStore, key kvstore.Realm) *PruningIndex {
	return &PruningIndex{
		kv:  kv,
		key: key,
	}
}

func (e *PruningIndex) ShouldPrune(newIndex iotago.EpochIndex) bool {
	value, err := e.kv.Get(e.key)
	if err != nil {
		return true
	}

	lastPrunedEpoch, _, err := iotago.EpochIndexFromBytes(value)
	if err != nil {
		return true
	}

	return newIndex > lastPrunedEpoch
}

//
// func (e *EvictionIndex[K]) MarkEvicted(index K) (previous K, hadPrevious bool) {
// 	var prev K
// 	var hadPrev bool
//
// 	if e.index == nil {
// 		e.index = new(K)
// 		hadPrev = false
// 	} else {
// 		prev = *e.index
// 		hadPrev = true
// 	}
//
// 	*e.index = index
//
// 	return prev, hadPrev
// }
//
// func (e *EvictionIndex[K]) Index() (current K, valid bool) {
// 	if e.index == nil {
// 		return 0, false
// 	}
//
// 	return *e.index, true
// }
//
// func (e *EvictionIndex[K]) NextIndex() K {
// 	if e.index == nil {
// 		return 0
// 	}
//
// 	return *e.index + 1
// }
//
// func (e *EvictionIndex[K]) IsEvicted(index K) bool {
// 	if e.index == nil {
// 		return false
// 	}
//
// 	return index <= *e.index
// }
