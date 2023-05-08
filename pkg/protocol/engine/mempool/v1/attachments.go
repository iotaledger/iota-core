package mempoolv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

type AttachmentStatus uint8

const (
	AttachmentPending AttachmentStatus = iota
	AttachmentIncluded
	AttachmentOrphaned
)

type Attachments struct {
	attachments                *shrinkingmap.ShrinkingMap[iotago.BlockID, AttachmentStatus]
	earliestIncludedAttachment *promise.Value[iotago.BlockID]
	allAttachmentsEvicted      *promise.Event

	mutex sync.RWMutex
}

func NewAttachments() *Attachments {
	return &Attachments{
		attachments:                shrinkingmap.New[iotago.BlockID, AttachmentStatus](),
		earliestIncludedAttachment: promise.NewValue[iotago.BlockID](),
		allAttachmentsEvicted:      promise.NewEvent(),
	}
}

func (a *Attachments) Add(blockID iotago.BlockID) (added bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.attachments.Has(blockID) {
		return false
	}

	a.attachments.Set(blockID, AttachmentPending)

	return true
}

func (a *Attachments) MarkIncluded(blockID iotago.BlockID) (included bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.attachments.Set(blockID, AttachmentIncluded)

	if lowestSlotIndex := a.earliestIncludedAttachment.Get(); lowestSlotIndex.Index() == 0 || blockID.Index() < lowestSlotIndex.Index() {
		a.earliestIncludedAttachment.Set(blockID)
	}

	return true
}

func (a *Attachments) MarkOrphaned(blockID iotago.BlockID) (orphaned bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	previousState, exists := a.attachments.Get(blockID)
	if !exists || previousState == AttachmentOrphaned {
		return false
	}

	a.attachments.Set(blockID, AttachmentOrphaned)

	if previousState == AttachmentIncluded && blockID.Index() == a.earliestIncludedAttachment.Get().Index() {
		a.earliestIncludedAttachment.Set(a.findLowestIncludedAttachment())
	}

	return true
}

func (a *Attachments) EarliestIncludedAttachment() iotago.BlockID {
	return a.earliestIncludedAttachment.Get()
}

func (a *Attachments) OnEarliestIncludedAttachmentUpdated(callback func(id iotago.BlockID)) (unsubscribe func()) {
	return a.earliestIncludedAttachment.OnUpdate(callback)
}

func (a *Attachments) OnAllAttachmentsEvicted(callback func()) {
	a.allAttachmentsEvicted.OnTrigger(callback)
}

func (a *Attachments) Evict(id iotago.BlockID) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	if a.attachments.Delete(id) && a.attachments.IsEmpty() {
		a.allAttachmentsEvicted.Trigger()
	}
}

func (a *Attachments) WasIncluded() bool {
	return a.EarliestIncludedAttachment().Index() != 0
}

func (a *Attachments) findLowestIncludedAttachment() iotago.BlockID {
	//TODO: we might need a deterministic sort here
	var lowestIncludedAttachment iotago.BlockID

	a.attachments.ForEach(func(blockID iotago.BlockID, status AttachmentStatus) bool {
		if status == AttachmentIncluded && (lowestIncludedAttachment.Index() == 0 || blockID.Index() < lowestIncludedAttachment.Index()) {
			lowestIncludedAttachment = blockID
		}

		return true
	})

	return lowestIncludedAttachment
}
