package trivialsyncmanager

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
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
	events *syncmanager.Events

	lastAcceptedBlockID     iotago.BlockID
	lastAcceptedIssuingTime time.Time
	lastAcceptedBlockIDLock syncutils.RWMutex

	lastConfirmedBlockID     iotago.BlockID
	lastConfirmedIssuingTime time.Time
	lastConfirmedBlockIDLock syncutils.RWMutex

	latestCommitment     *model.Commitment
	latestCommitmentLock syncutils.RWMutex

	latestFinalizedSlot     iotago.SlotIndex
	latestFinalizedSlotLock syncutils.RWMutex

	isBootstrappedFunc isBootstrappedFunc

	module.Module
}

// NewProvider creates a new SyncManager provider.
func NewProvider() module.Provider[*engine.Engine, syncmanager.SyncManager] {
	return module.Provide(func(e *engine.Engine) syncmanager.SyncManager {
		s := New(e.IsBootstrapped, e.Storage.Settings().LatestCommitment(), e.Storage.Settings().LatestFinalizedSlot()) //TODO: handle changes to the bootstrapped state to trigger updates
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("SyncManager", 1))

		e.Events.BlockGadget.BlockAccepted.Hook(func(b *blocks.Block) {
			if s.updateLastAcceptedBlock(b.ID(), b.IssuingTime()) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			if s.updateLastConfirmedBlock(b.ID(), b.IssuingTime()) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
			if s.updateLatestCommitment(details.Commitment) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
			if s.updateFinalizedSlot(index) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		s.TriggerInitialized()

		return s
	})
}

func New(bootstrappedFunc isBootstrappedFunc, latestCommitment *model.Commitment, finalizedSlot iotago.SlotIndex) *SyncManager {
	return &SyncManager{
		events:               syncmanager.NewEvents(),
		isBootstrappedFunc:   bootstrappedFunc,
		lastAcceptedBlockID:  iotago.EmptyBlockID(),
		lastConfirmedBlockID: iotago.EmptyBlockID(),
		latestCommitment:     latestCommitment,
		latestFinalizedSlot:  finalizedSlot,
	}
}

func (s *SyncManager) SyncStatus() *syncmanager.SyncStatus {
	s.lastAcceptedBlockIDLock.RLock()
	s.lastConfirmedBlockIDLock.RLock()
	s.latestCommitmentLock.RLock()
	s.latestFinalizedSlotLock.RLock()
	defer s.lastAcceptedBlockIDLock.RUnlock()
	defer s.lastConfirmedBlockIDLock.RUnlock()
	defer s.latestCommitmentLock.RUnlock()
	defer s.latestFinalizedSlotLock.RUnlock()

	return &syncmanager.SyncStatus{
		NodeSynced:           s.isBootstrappedFunc(),
		LastAcceptedBlockID:  s.lastAcceptedBlockID,
		LastConfirmedBlockID: s.lastConfirmedBlockID,
		LatestCommitment:     s.latestCommitment,
		LatestFinalizedSlot:  s.latestFinalizedSlot,
	}
}

func (s *SyncManager) Shutdown() {
	s.TriggerStopped()
}

func (s *SyncManager) updateLastAcceptedBlock(id iotago.BlockID, issuingTime time.Time) (changed bool) {
	s.lastAcceptedBlockIDLock.Lock()
	defer s.lastAcceptedBlockIDLock.Unlock()

	if s.lastAcceptedIssuingTime.After(issuingTime) {
		return false
	}

	s.lastAcceptedBlockID = id

	return true
}

func (s *SyncManager) updateLastConfirmedBlock(id iotago.BlockID, issuingTime time.Time) (changed bool) {
	s.lastConfirmedBlockIDLock.Lock()
	defer s.lastConfirmedBlockIDLock.Unlock()

	if s.lastConfirmedIssuingTime.After(issuingTime) {
		return false
	}

	s.lastConfirmedBlockID = id

	return true
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

func (s *SyncManager) updateFinalizedSlot(index iotago.SlotIndex) (changed bool) {
	s.latestFinalizedSlotLock.Lock()
	defer s.latestFinalizedSlotLock.Unlock()

	if s.latestFinalizedSlot != index {
		s.latestFinalizedSlot = index
		return true
	}

	return false
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

func (s *SyncManager) triggerUpdate() {
	s.events.UpdatedStatus.Trigger(s.SyncStatus())
}
