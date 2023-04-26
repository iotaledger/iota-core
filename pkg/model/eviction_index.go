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
	var prev iotago.SlotIndex
	var hadPrev bool

	if e.index == nil {
		e.index = new(iotago.SlotIndex)
		hadPrev = false
	} else {
		prev = *e.index
		hadPrev = true
	}

	*e.index = index

	return prev, hadPrev
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

func (e EvictionIndex) IsEvicted(index iotago.SlotIndex) bool {
	if e.index == nil {
		return false
	}

	return index <= *e.index
}
