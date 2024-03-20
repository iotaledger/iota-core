package trivialsyncmanager

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type (
	isBootstrappedFunc func(e *engine.Engine) bool
)

type SyncManager struct {
	events        *syncmanager.Events
	engine        *engine.Engine
	syncThreshold time.Duration

	lastAcceptedBlockSlot     iotago.SlotIndex
	lastAcceptedBlockSlotLock syncutils.RWMutex

	lastConfirmedBlockSlot     iotago.SlotIndex
	lastConfirmedBlockSlotLock syncutils.RWMutex

	latestCommitment     *model.Commitment
	latestCommitmentLock syncutils.RWMutex

	latestFinalizedSlot     iotago.SlotIndex
	latestFinalizedSlotLock syncutils.RWMutex

	lastPrunedEpoch     iotago.EpochIndex
	hasPruned           bool
	lastPrunedEpochLock syncutils.RWMutex

	isSynced     bool
	isSyncedLock syncutils.RWMutex

	isFinalizationDelayed     bool
	isFinalizationDelayedLock syncutils.RWMutex

	isBootstrapped     bool
	isBootstrappedLock syncutils.RWMutex

	optsIsBootstrappedFunc    isBootstrappedFunc
	optsBootstrappedThreshold time.Duration

	module.Module
}

// NewProvider creates a new SyncManager provider.
func NewProvider(opts ...options.Option[SyncManager]) module.Provider[*engine.Engine, syncmanager.SyncManager] {
	return module.Provide(func(e *engine.Engine) syncmanager.SyncManager {
		s := New(e.NewSubModule("SyncManager"), e, e.Storage.Settings().LatestCommitment(), e.Storage.Settings().LatestFinalizedSlot(), opts...)
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("SyncManager", workerpool.WithWorkerCount(1)))

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

			syncChanged := s.updateSyncStatus()
			commitmentChanged := s.updateLatestCommitment(commitment)

			if syncChanged || commitmentChanged {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
			if s.updateFinalizedSlot(slot) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Storage.Pruned.Hook(func(epoch iotago.EpochIndex) {
			if s.updatePrunedEpoch(epoch, true) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SyncManager.LinkTo(s.events)

		return s
	})
}

func New(subModule module.Module, e *engine.Engine, latestCommitment *model.Commitment, finalizedSlot iotago.SlotIndex, opts ...options.Option[SyncManager]) *SyncManager {
	return module.InitSimpleLifecycle(options.Apply(&SyncManager{
		Module:                 subModule,
		events:                 syncmanager.NewEvents(),
		engine:                 e,
		syncThreshold:          10 * time.Second,
		lastAcceptedBlockSlot:  latestCommitment.Slot(),
		lastConfirmedBlockSlot: latestCommitment.Slot(),
		latestCommitment:       latestCommitment,
		latestFinalizedSlot:    finalizedSlot,
		lastPrunedEpoch:        0,
		hasPruned:              false,
		isSynced:               false,
		isFinalizationDelayed:  true,
		isBootstrapped:         false,

		optsBootstrappedThreshold: 10 * time.Second,
	}, opts, func(s *SyncManager) {
		s.updatePrunedEpoch(s.engine.Storage.LastPrunedEpoch())

		// set the default bootstrapped function
		if s.optsIsBootstrappedFunc == nil {
			s.optsIsBootstrappedFunc = func(e *engine.Engine) bool {
				return time.Since(e.Clock.Accepted().RelativeTime()) < s.optsBootstrappedThreshold && e.Notarization.IsBootstrapped()
			}
		}
	}))
}

func (s *SyncManager) SyncStatus() *syncmanager.SyncStatus {
	// get all the locks so we have an atomic view of the state
	s.isBootstrappedLock.RLock()
	s.isSyncedLock.RLock()
	s.isFinalizationDelayedLock.RLock()
	s.lastAcceptedBlockSlotLock.RLock()
	s.lastConfirmedBlockSlotLock.RLock()
	s.latestCommitmentLock.RLock()
	s.latestFinalizedSlotLock.RLock()
	s.lastPrunedEpochLock.RLock()

	defer func() {
		s.isBootstrappedLock.RUnlock()
		s.isSyncedLock.RUnlock()
		s.isFinalizationDelayedLock.RUnlock()
		s.lastAcceptedBlockSlotLock.RUnlock()
		s.lastConfirmedBlockSlotLock.RUnlock()
		s.latestCommitmentLock.RUnlock()
		s.latestFinalizedSlotLock.RUnlock()
		s.lastPrunedEpochLock.RUnlock()
	}()

	return &syncmanager.SyncStatus{
		NodeBootstrapped:       s.isBootstrapped,
		NodeSynced:             s.isSynced,
		FinalizationDelayed:    s.isFinalizationDelayed,
		LastAcceptedBlockSlot:  s.lastAcceptedBlockSlot,
		LastConfirmedBlockSlot: s.lastConfirmedBlockSlot,
		LatestCommitment:       s.latestCommitment,
		LatestFinalizedSlot:    s.latestFinalizedSlot,
		LastPrunedEpoch:        s.lastPrunedEpoch,
		HasPruned:              s.hasPruned,
	}
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (s *SyncManager) Reset() {
	s.lastAcceptedBlockSlotLock.Lock()
	s.lastConfirmedBlockSlotLock.Lock()
	s.latestCommitmentLock.RLock()
	s.isSyncedLock.Lock()
	s.isFinalizationDelayedLock.Lock()
	defer s.lastAcceptedBlockSlotLock.Unlock()
	defer s.lastConfirmedBlockSlotLock.Unlock()
	defer s.latestCommitmentLock.RUnlock()
	defer s.isSyncedLock.Unlock()
	defer s.isFinalizationDelayedLock.Unlock()

	s.lastAcceptedBlockSlot = s.latestCommitment.Slot()
	s.lastConfirmedBlockSlot = s.latestCommitment.Slot()
	// Mark the synced flag as false,
	// because we clear the latest accepted blocks and return the whole state to the last committed slot.
	s.isSynced = false
	s.isFinalizationDelayed = true
}

func (s *SyncManager) updateLastAcceptedBlock(id iotago.BlockID) (changed bool) {
	s.lastAcceptedBlockSlotLock.Lock()
	defer s.lastAcceptedBlockSlotLock.Unlock()

	if id.Slot() > s.lastAcceptedBlockSlot {
		s.lastAcceptedBlockSlot = id.Slot()
		return true
	}

	return false
}

func (s *SyncManager) updateLastConfirmedBlock(id iotago.BlockID) (changed bool) {
	s.lastConfirmedBlockSlotLock.Lock()
	defer s.lastConfirmedBlockSlotLock.Unlock()

	if id.Slot() > s.lastConfirmedBlockSlot {
		s.lastConfirmedBlockSlot = id.Slot()
		return true
	}

	return false
}

func (s *SyncManager) updateLatestCommitment(commitment *model.Commitment) (changed bool) {
	s.latestCommitmentLock.Lock()

	if s.latestCommitment != commitment {
		s.latestCommitment = commitment
		s.latestCommitmentLock.Unlock()

		s.setIsFinalizationDelayed(s.LatestFinalizedSlot(), commitment.Slot())

		return true
	}
	s.latestCommitmentLock.Unlock()

	return false
}

func (s *SyncManager) updateBootstrappedStatus() {
	s.isBootstrappedLock.Lock()
	defer s.isBootstrappedLock.Unlock()

	if !s.isBootstrapped && s.optsIsBootstrappedFunc(s.engine) {
		s.isBootstrapped = true
	}
}

func (s *SyncManager) updateSyncStatus() (changed bool) {
	s.isSyncedLock.Lock()
	defer s.isSyncedLock.Unlock()

	if s.isSynced != (s.isBootstrapped && time.Since(s.engine.Clock.Accepted().RelativeTime()) < s.syncThreshold) {
		s.isSynced = !s.isSynced
		return true
	}

	return false
}

func (s *SyncManager) updateFinalizedSlot(slot iotago.SlotIndex) (changed bool) {
	s.latestFinalizedSlotLock.Lock()

	if s.latestFinalizedSlot != slot {
		s.latestFinalizedSlot = slot
		s.latestFinalizedSlotLock.Unlock()

		s.setIsFinalizationDelayed(slot, s.LatestCommitment().Slot())

		return true
	}
	s.latestFinalizedSlotLock.Unlock()

	return false
}

func (s *SyncManager) updatePrunedEpoch(epoch iotago.EpochIndex, hasPruned bool) (changed bool) {
	s.lastPrunedEpochLock.Lock()
	defer s.lastPrunedEpochLock.Unlock()

	if s.lastPrunedEpoch != epoch {
		s.lastPrunedEpoch = epoch
		s.hasPruned = hasPruned

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

func (s *SyncManager) setIsFinalizationDelayed(latestFinalizedSlot iotago.SlotIndex, latestCommitmentSlot iotago.SlotIndex) {
	s.isFinalizationDelayedLock.Lock()
	defer s.isFinalizationDelayedLock.Unlock()

	if latestCommitmentSlot < latestFinalizedSlot {
		// This should never happen, but if it does, we don't want to panic.
		return
	}

	s.isFinalizationDelayed = latestCommitmentSlot-latestFinalizedSlot > s.engine.CommittedAPI().ProtocolParameters().MaxCommittableAge()
}

func (s *SyncManager) IsFinalizationDelayed() bool {
	s.isFinalizationDelayedLock.RLock()
	defer s.isFinalizationDelayedLock.RUnlock()

	return s.isFinalizationDelayed
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

func (s *SyncManager) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	s.lastPrunedEpochLock.RLock()
	defer s.lastPrunedEpochLock.RUnlock()

	return s.lastPrunedEpoch, s.hasPruned
}

func (s *SyncManager) triggerUpdate() {
	s.events.UpdatedStatus.Trigger(s.SyncStatus())
}

func WithBootstrappedThreshold(threshold time.Duration) options.Option[SyncManager] {
	return func(s *SyncManager) {
		s.optsBootstrappedThreshold = threshold
	}
}

func WithBootstrappedFunc(isBootstrapped func(*engine.Engine) bool) options.Option[SyncManager] {
	return func(s *SyncManager) {
		s.optsIsBootstrappedFunc = isBootstrapped
	}
}
