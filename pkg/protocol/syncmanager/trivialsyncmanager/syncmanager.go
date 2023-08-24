package trivialsyncmanager

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/syncmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type (
	isBootstrappedFunc       func() bool
	relativeAcceptedTimeFunc func() clock.RelativeTime
)

type SyncManager struct {
	events        *syncmanager.Events
	syncThreshold time.Duration

	lastAcceptedBlockSlot     iotago.SlotIndex
	lastAcceptedBlockSlotLock syncutils.RWMutex

	lastConfirmedBlockSlot     iotago.SlotIndex
	lastConfirmedBlockSlotLock syncutils.RWMutex

	latestCommitment     *model.Commitment
	latestCommitmentLock syncutils.RWMutex

	latestFinalizedSlot     iotago.SlotIndex
	latestFinalizedSlotLock syncutils.RWMutex

	lastPrunedSlot     iotago.SlotIndex
	lastPrunedSlotLock syncutils.RWMutex

	isSynced     bool
	isSyncedLock syncutils.RWMutex

	isBootstrapped     bool
	isBootstrappedLock syncutils.RWMutex

	isBootstrappedFunc       isBootstrappedFunc
	relativeAcceptedTimeFunc relativeAcceptedTimeFunc

	module.Module
}

// NewProvider creates a new SyncManager provider.
func NewProvider() module.Provider[*engine.Engine, syncmanager.SyncManager] {
	return module.Provide(func(e *engine.Engine) syncmanager.SyncManager {
		s := New(e.IsBootstrapped, e.Clock.Accepted, e.Storage.Settings().LatestCommitment(), e.Storage.Settings().LatestFinalizedSlot())
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("SyncManager", 1))

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			if s.updateLastAcceptedBlock(b.ID()) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			if s.updateLastConfirmedBlock(b.ID()) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
			if !s.IsBootstrapped() {
				s.updateBootstrappedStatus()
			}

			syncChaged := s.updateSyncStatus()
			commitmentChanged := s.updateLatestCommitment(commitment)

			if syncChaged || commitmentChanged {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
			if s.updateFinalizedSlot(index) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.StoragePruned.Hook(func(index iotago.SlotIndex) {
			if s.updatePrunedSlot(index) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SyncManager.LinkTo(s.events)
		s.TriggerInitialized()

		return s
	})
}

func New(bootstrappedFunc isBootstrappedFunc, acceptedTimeFunc relativeAcceptedTimeFunc, latestCommitment *model.Commitment, finalizedSlot iotago.SlotIndex) *SyncManager {
	return &SyncManager{
		events:                   syncmanager.NewEvents(),
		syncThreshold:            10 * time.Second,
		isBootstrappedFunc:       bootstrappedFunc,
		relativeAcceptedTimeFunc: acceptedTimeFunc,
		lastAcceptedBlockSlot:    latestCommitment.Index(),
		lastConfirmedBlockSlot:   latestCommitment.Index(),
		latestCommitment:         latestCommitment,
		latestFinalizedSlot:      finalizedSlot,
	}
}

func (s *SyncManager) SyncStatus() *syncmanager.SyncStatus {
	s.lastAcceptedBlockSlotLock.RLock()
	s.lastConfirmedBlockSlotLock.RLock()
	s.latestCommitmentLock.RLock()
	s.latestFinalizedSlotLock.RLock()
	s.lastPrunedSlotLock.RLock()
	defer s.lastAcceptedBlockSlotLock.RUnlock()
	defer s.lastConfirmedBlockSlotLock.RUnlock()
	defer s.latestCommitmentLock.RUnlock()
	defer s.latestFinalizedSlotLock.RUnlock()
	defer s.lastPrunedSlotLock.RUnlock()

	return &syncmanager.SyncStatus{
		NodeSynced:             s.IsNodeSynced(),
		LastAcceptedBlockSlot:  s.lastAcceptedBlockSlot,
		LastConfirmedBlockSlot: s.lastConfirmedBlockSlot,
		LatestCommitment:       s.latestCommitment,
		LatestFinalizedSlot:    s.latestFinalizedSlot,
		LatestPrunedSlot:       s.lastPrunedSlot,
	}
}

func (s *SyncManager) Shutdown() {
	s.TriggerStopped()
}

func (s *SyncManager) updateLastAcceptedBlock(id iotago.BlockID) (changed bool) {
	s.lastAcceptedBlockSlotLock.Lock()
	defer s.lastAcceptedBlockSlotLock.Unlock()

	if id.Index() > s.lastAcceptedBlockSlot {
		s.lastAcceptedBlockSlot = id.Index()
		return true
	}

	return false
}

func (s *SyncManager) updateLastConfirmedBlock(id iotago.BlockID) (changed bool) {
	s.lastConfirmedBlockSlotLock.Lock()
	defer s.lastConfirmedBlockSlotLock.Unlock()

	if id.Index() > s.lastConfirmedBlockSlot {
		s.lastConfirmedBlockSlot = id.Index()
		return true
	}

	return false
}

func (s *SyncManager) updateLatestCommitment(commitment *model.Commitment) (changed bool) {
	s.latestCommitmentLock.Lock()
	defer s.latestCommitmentLock.Unlock()

	if s.latestCommitment != commitment {
		s.latestCommitment = commitment
		return true
	}

	return false
}

func (s *SyncManager) updateBootstrappedStatus() {
	s.isBootstrappedLock.Lock()
	defer s.isBootstrappedLock.Unlock()

	if !s.isBootstrapped && s.isBootstrappedFunc() {
		s.isBootstrapped = true
	}
}

func (s *SyncManager) updateSyncStatus() (changed bool) {
	s.isSyncedLock.Lock()
	defer s.isSyncedLock.Unlock()

	if s.isSynced != (s.isBootstrapped && time.Since(s.relativeAcceptedTimeFunc().RelativeTime()) < s.syncThreshold) {
		s.isSynced = !s.isSynced
		return true
	}

	return false
}

func (s *SyncManager) updateFinalizedSlot(index iotago.SlotIndex) (changed bool) {
	s.latestFinalizedSlotLock.Lock()
	defer s.latestFinalizedSlotLock.Unlock()

	if s.latestFinalizedSlot != index {
		s.latestFinalizedSlot = index
		return true
	}

	return false
}

func (s *SyncManager) updatePrunedSlot(index iotago.SlotIndex) (changed bool) {
	s.lastPrunedSlotLock.Lock()
	defer s.lastPrunedSlotLock.Unlock()

	if s.lastPrunedSlot != index {
		s.lastPrunedSlot = index
		return true
	}

	return false
}

func (s *SyncManager) IsBootstrapped() bool {
	s.isBootstrappedLock.RLock()
	defer s.isBootstrappedLock.RUnlock()

	return s.isBootstrapped
}

func (s *SyncManager) IsNodeSynced() bool {
	s.isSyncedLock.RLock()
	defer s.isSyncedLock.RUnlock()

	return s.isSynced
}

func (s *SyncManager) LastAcceptedBlockSlot() iotago.SlotIndex {
	s.lastAcceptedBlockSlotLock.RLock()
	defer s.lastAcceptedBlockSlotLock.RUnlock()

	return s.lastAcceptedBlockSlot
}

func (s *SyncManager) LastConfirmedBlockSlot() iotago.SlotIndex {
	s.lastConfirmedBlockSlotLock.RLock()
	defer s.lastConfirmedBlockSlotLock.RUnlock()

	return s.lastConfirmedBlockSlot
}

func (s *SyncManager) LatestCommitment() *model.Commitment {
	s.latestCommitmentLock.RLock()
	defer s.latestCommitmentLock.RUnlock()

	return s.latestCommitment
}

func (s *SyncManager) LatestFinalizedSlot() iotago.SlotIndex {
	s.latestFinalizedSlotLock.RLock()
	defer s.latestFinalizedSlotLock.RUnlock()

	return s.latestFinalizedSlot
}

func (s *SyncManager) LastPrunedSlot() iotago.SlotIndex {
	s.lastPrunedSlotLock.RLock()
	defer s.lastPrunedSlotLock.RUnlock()

	return s.lastPrunedSlot
}

func (s *SyncManager) triggerUpdate() {
	s.events.UpdatedStatus.Trigger(s.SyncStatus())
}
