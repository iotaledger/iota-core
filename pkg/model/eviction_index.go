package model

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type EvictionIndex[K iotago.SlotIndex | iotago.EpochIndex] struct {
	index *K
}

func NewEvictionIndex[K iotago.SlotIndex | iotago.EpochIndex]() *EvictionIndex[K] {
	return &EvictionIndex[K]{}
}

func (e *EvictionIndex[K]) ShouldEvict(newIndex K) bool {
	if e.index == nil {
		return true
	}

	return newIndex > *e.index
}

func (e *EvictionIndex[K]) MarkEvicted(index K) (previous K, hadPrevious bool) {
	var prev K
	var hadPrev bool

	if e.index == nil {
		e.index = new(K)
		hadPrev = false
	} else {
		prev = *e.index
		hadPrev = true
	}

	*e.index = index

	return prev, hadPrev
}

func (e *EvictionIndex[K]) Index() (current K, valid bool) {
	if e.index == nil {
		return 0, false
	}

	return *e.index, true
}

func (e *EvictionIndex[K]) NextIndex() K {
	if e.index == nil {
		return 0
	}

	return *e.index + 1
}

func (e *EvictionIndex[K]) IsEvicted(index K) bool {
	if e.index == nil {
		return false
	}

	return index <= *e.index
}
