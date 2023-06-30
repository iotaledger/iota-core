package trivialsyncmanager

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type (
	isBootstrappedFunc func() bool
)

type SyncManager struct {
	lastAcceptedBlockID     iotago.BlockID
	lastAcceptedIssuingTime time.Time
	lastAcceptedBlockIDLock syncutils.RWMutex

	lastConfirmedBlockID     iotago.BlockID
	lastConfirmedIssuingTime time.Time
	lastConfirmedBlockIDLock syncutils.RWMutex

	finalizedSlot     iotago.SlotIndex
	finalizedSlotLock syncutils.RWMutex

	latestCommittedSlot     iotago.SlotIndex
	latestCommittedSlotLock syncutils.RWMutex

	isBootstrappedFunc isBootstrappedFunc

	module.Module
}

// NewProvider creates a new SyncManager provider.
func NewProvider() module.Provider[*engine.Engine, syncmanager.SyncManager] {
	return module.Provide(func(e *engine.Engine) syncmanager.SyncManager {
		s := New(e.IsBootstrapped)
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("SyncManager", 1))

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			s.updateLastAcceptedBlock(b.ID(), b.IssuingTime())
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			s.updateLastConfirmedBlock(b.ID(), b.IssuingTime())
		}, asyncOpt)

		e.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
			s.updateLatestCommittedSlot(details.Commitment.Index())
		}, asyncOpt)

		e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
			s.updateFinalizedSlot(index)
		})

		s.TriggerInitialized()

		return s
	})
}

func New(bootstrappedFunc isBootstrappedFunc) *SyncManager {
	return &SyncManager{
		isBootstrappedFunc:   bootstrappedFunc,
		lastAcceptedBlockID:  iotago.EmptyBlockID(),
		lastConfirmedBlockID: iotago.EmptyBlockID(),
	}
}

func (s *SyncManager) SyncStatus() *syncmanager.SyncStatus {
	s.lastAcceptedBlockIDLock.RLock()
	s.lastConfirmedBlockIDLock.RLock()
	s.finalizedSlotLock.RLock()
	s.latestCommittedSlotLock.RLock()
	defer s.lastAcceptedBlockIDLock.RUnlock()
	defer s.lastConfirmedBlockIDLock.RUnlock()
	defer s.finalizedSlotLock.RUnlock()
	defer s.latestCommittedSlotLock.RUnlock()

	return &syncmanager.SyncStatus{
		NodeSynced:           s.isBootstrappedFunc(),
		LastAcceptedBlockID:  s.lastAcceptedBlockID,
		LastConfirmedBlockID: s.lastConfirmedBlockID,
		FinalizedSlot:        s.finalizedSlot,
		LatestCommittedSlot:  s.latestCommittedSlot,
	}
}

func (s *SyncManager) Shutdown() {
	s.TriggerStopped()
}

func (s *SyncManager) updateLastAcceptedBlock(id iotago.BlockID, issuingTime time.Time) {
	s.lastAcceptedBlockIDLock.Lock()
	defer s.lastAcceptedBlockIDLock.Unlock()

	if s.lastAcceptedIssuingTime.After(issuingTime) {
		return
	}

	s.lastAcceptedBlockID = id
}

func (s *SyncManager) updateLastConfirmedBlock(id iotago.BlockID, issuingTime time.Time) {
	s.lastConfirmedBlockIDLock.Lock()
	defer s.lastConfirmedBlockIDLock.Unlock()

	if s.lastConfirmedIssuingTime.After(issuingTime) {
		return
	}

	s.lastConfirmedBlockID = id
}

func (s *SyncManager) updateFinalizedSlot(index iotago.SlotIndex) {
	s.finalizedSlotLock.Lock()
	defer s.finalizedSlotLock.Unlock()

	s.finalizedSlot = index
}

func (s *SyncManager) updateLatestCommittedSlot(index iotago.SlotIndex) {
	s.latestCommittedSlotLock.Lock()
	defer s.latestCommittedSlotLock.Unlock()

	s.latestCommittedSlot = index
}

func (s *SyncManager) IsNodeSynced() bool {
	return s.isBootstrappedFunc()
}

func (s *SyncManager) LastAcceptedBlock() iotago.BlockID {
	s.lastAcceptedBlockIDLock.RLock()
	defer s.lastAcceptedBlockIDLock.RUnlock()

	return s.lastAcceptedBlockID
}

func (s *SyncManager) LastConfirmedBlock() iotago.BlockID {
	s.lastConfirmedBlockIDLock.RLock()
	defer s.lastConfirmedBlockIDLock.RUnlock()

	return s.lastConfirmedBlockID
}

func (s *SyncManager) FinalizedSlot() iotago.SlotIndex {
	s.finalizedSlotLock.RLock()
	defer s.finalizedSlotLock.RUnlock()

	return s.finalizedSlot
}

func (s *SyncManager) LatestCommittedSlot() iotago.SlotIndex {
	s.latestCommittedSlotLock.RLock()
	defer s.latestCommittedSlotLock.RUnlock()

	return s.latestCommittedSlot
}
