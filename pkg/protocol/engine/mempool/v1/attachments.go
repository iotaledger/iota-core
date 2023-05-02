package mempoolv1

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Attachments struct {
	includedAttachments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *advancedset.AdvancedSet[iotago.BlockID]]
	pendingAttachments  *advancedset.AdvancedSet[iotago.BlockID]

	lowestIncludedSlotIndex *promise.Value[iotago.SlotIndex]

	mutex sync.RWMutex
}

func (l *Attachments) Add(blockID iotago.BlockID) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	return l.pendingAttachments.Add(blockID)
}

func (l *Attachments) MarkIncluded(blockID iotago.BlockID) (included bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.pendingAttachments.Delete(blockID)

	attachments, _ := l.includedAttachments.GetOrCreate(blockID.Index(), lo.NoVariadic(advancedset.New[iotago.BlockID]))
	if included = attachments.Add(blockID); included {
		if lowestSlotIndex := l.lowestIncludedSlotIndex.Get(); lowestSlotIndex == 0 || blockID.Index() < lowestSlotIndex {
			l.lowestIncludedSlotIndex.Set(blockID.Index())
		}
	}

	return included
}

func (l *Attachments) MarkOrphaned(blockID iotago.BlockID) (orphaned bool) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if l.pendingAttachments.Delete(blockID) {
		return true
	}

	includedAttachments, exists := l.includedAttachments.Get(blockID.Index())

	return true
}
