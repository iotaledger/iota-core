package mempoolv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Attachments struct {
	attachments           *shrinkingmap.ShrinkingMap[iotago.BlockID, bool]
	earliestIncludedSlot  *promise.Value[iotago.SlotIndex]
	allAttachmentsEvicted *promise.Event

	mutex sync.RWMutex
}

func NewAttachments() *Attachments {
	return &Attachments{
		attachments:           shrinkingmap.New[iotago.BlockID, bool](),
		earliestIncludedSlot:  promise.NewValue[iotago.SlotIndex](),
		allAttachmentsEvicted: promise.NewEvent(),
	}
}

func (a *Attachments) Add(blockID iotago.BlockID) (added bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.attachments.Has(blockID) {
		return false
	}

	a.attachments.Set(blockID, false)

	return true
}

func (a *Attachments) MarkIncluded(blockID iotago.BlockID) (included bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.attachments.Set(blockID, true)

	if lowestSlotIndex := a.earliestIncludedSlot.Get(); lowestSlotIndex == 0 || blockID.Index() < lowestSlotIndex {
		a.earliestIncludedSlot.Set(blockID.Index())
	}

	return true
}

func (a *Attachments) MarkOrphaned(blockID iotago.BlockID) (orphaned bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	previousState, exists := a.attachments.Get(blockID)
	if !exists {
		return false
	}

	a.evict(blockID)

	if previousState && blockID.Index() == a.earliestIncludedSlot.Get() {
		a.earliestIncludedSlot.Set(a.findLowestIncludedSlotIndex())
	}

	return true
}

func (a *Attachments) EarliestIncludedSlot() iotago.SlotIndex {
	return a.earliestIncludedSlot.Get()
}

func (a *Attachments) OnEarliestIncludedSlotUpdated(callback func(prevIndex, newIndex iotago.SlotIndex)) (unsubscribe func()) {
	return a.earliestIncludedSlot.OnUpdate(callback)
}

func (a *Attachments) OnAllAttachmentsEvicted(callback func()) {
	a.allAttachmentsEvicted.OnTrigger(callback)
}

func (a *Attachments) Evict(id iotago.BlockID) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.evict(id)
}

func (a *Attachments) evict(id iotago.BlockID) {
	if a.attachments.Delete(id) && a.attachments.IsEmpty() {
		a.allAttachmentsEvicted.Trigger()
	}
}

func (a *Attachments) findLowestIncludedSlotIndex() iotago.SlotIndex {
	var lowestIncludedSlotIndex iotago.SlotIndex

	a.attachments.ForEach(func(blockID iotago.BlockID, included bool) bool {
		if included && (lowestIncludedSlotIndex == 0 || blockID.Index() < lowestIncludedSlotIndex) {
			lowestIncludedSlotIndex = blockID.Index()
		}

		return true
	})

	return lowestIncludedSlotIndex
}
