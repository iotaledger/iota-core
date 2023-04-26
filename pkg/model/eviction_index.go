package model

import iotago "github.com/iotaledger/iota.go/v4"

type EvictionIndex struct {
	index *iotago.SlotIndex
}

func (e EvictionIndex) ShouldEvict(newIndex iotago.SlotIndex) bool {
	if e.index == nil {
		return true
	}

	return newIndex > *e.index
}

func (e EvictionIndex) MarkEvicted(index iotago.SlotIndex) (previous iotago.SlotIndex, hadPrevious bool) {
	if e.index == nil {
		e.index = new(iotago.SlotIndex)
		return 0, false
	}
	prev := *e.index
	*e.index = index

	return prev, true
}

func (e EvictionIndex) Index() (current iotago.SlotIndex, valid bool) {
	if e.index == nil {
		return 0, false
	}

	return *e.index, true
}

func (e EvictionIndex) NextIndex() iotago.SlotIndex {
	if e.index == nil {
		return 0
	}

	return *e.index + 1
}
