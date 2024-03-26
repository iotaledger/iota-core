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

func WithSyncThreshold(threshold time.Duration) options.Option[SyncManager] {
	return func(s *SyncManager) {
		s.optsSyncThreshold = threshold
	}
}

func WithIsBootstrappedFunc(isBootstrapped isBootstrappedFunc) options.Option[SyncManager] {
	return func(s *SyncManager) {
		s.optsIsBootstrappedFunc = isBootstrapped
	}
}

func WithBootstrappedThreshold(threshold time.Duration) options.Option[SyncManager] {
	return func(s *SyncManager) {
		s.optsBootstrappedThreshold = threshold
	}
}

type SyncManager struct {
	events *syncmanager.Events
	engine *engine.Engine

	// options
	optsSyncThreshold         time.Duration
	optsIsBootstrappedFunc    isBootstrappedFunc
	optsBootstrappedThreshold time.Duration

	// state
	isBootstrappedLock syncutils.RWMutex
	isBootstrapped     bool

	isSyncedLock syncutils.RWMutex
	isSynced     bool

	isFinalizationDelayedLock syncutils.RWMutex
	isFinalizationDelayed     bool

	lastAcceptedBlockSlotLock syncutils.RWMutex
	lastAcceptedBlockSlot     iotago.SlotIndex

	lastConfirmedBlockSlotLock syncutils.RWMutex
	lastConfirmedBlockSlot     iotago.SlotIndex

	latestCommitmentLock syncutils.RWMutex
	latestCommitment     *model.Commitment

	latestFinalizedSlotLock syncutils.RWMutex
	latestFinalizedSlot     iotago.SlotIndex

	lastPrunedEpochLock syncutils.RWMutex
	lastPrunedEpoch     iotago.EpochIndex
	hasPruned           bool

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
			var bootstrapChanged bool
			if !s.IsBootstrapped() {
				bootstrapChanged = s.updateBootstrappedStatus()
			}

			syncChanged := s.updateSyncStatus()
			commitmentChanged := s.updateLatestCommitment(commitment)

			if bootstrapChanged || syncChanged || commitmentChanged {
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
		Module: subModule,
		events: syncmanager.NewEvents(),
		engine: e,

		optsSyncThreshold:         10 * time.Second,
		optsIsBootstrappedFunc:    nil,
		optsBootstrappedThreshold: 10 * time.Second,

		isBootstrapped:         false,
		isSynced:               false,
		isFinalizationDelayed:  true,
		lastAcceptedBlockSlot:  latestCommitment.Slot(),
		lastConfirmedBlockSlot: latestCommitment.Slot(),
		latestCommitment:       latestCommitment,
		latestFinalizedSlot:    finalizedSlot,
		lastPrunedEpoch:        0,
		hasPruned:              false,
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
	s.isSyncedLock.Lock()
	s.isFinalizationDelayedLock.Lock()
	s.lastAcceptedBlockSlotLock.Lock()
	s.lastConfirmedBlockSlotLock.Lock()
	s.latestCommitmentLock.RLock()

	defer func() {
		s.isSyncedLock.Unlock()
		s.isFinalizationDelayedLock.Unlock()
		s.lastAcceptedBlockSlotLock.Unlock()
		s.lastConfirmedBlockSlotLock.Unlock()
		s.latestCommitmentLock.RUnlock()
	}()

	// Mark the synced flag as false,
	// because we clear the latest accepted blocks and return the whole state to the last committed slot.
	s.isSynced = false
	s.isFinalizationDelayed = true
	s.lastAcceptedBlockSlot = s.latestCommitment.Slot()
	s.lastConfirmedBlockSlot = s.latestCommitment.Slot()
}

func (s *SyncManager) triggerUpdate() {
	s.events.UpdatedStatus.Trigger(s.SyncStatus())
}

func (s *SyncManager) updateBootstrappedStatus() (changed bool) {
	s.isBootstrappedLock.Lock()
	defer s.isBootstrappedLock.Unlock()

	if !s.isBootstrapped && s.optsIsBootstrappedFunc(s.engine) {
		s.isBootstrapped = true

		return true
	}

	return false
}

func (s *SyncManager) IsBootstrapped() bool {
	s.isBootstrappedLock.RLock()
	defer s.isBootstrappedLock.RUnlock()

	return s.isBootstrapped
}

func (s *SyncManager) updateSyncStatus() (changed bool) {
	s.isSyncedLock.Lock()
	defer s.isSyncedLock.Unlock()

	isSynced := s.isBootstrapped && time.Since(s.engine.Clock.Accepted().RelativeTime()) < s.optsSyncThreshold
	if s.isSynced != isSynced {
		s.isSynced = isSynced

		return true
	}

	return false
}

func (s *SyncManager) IsNodeSynced() bool {
	s.isSyncedLock.RLock()
	defer s.isSyncedLock.RUnlock()

	return s.isSynced
}

func (s *SyncManager) updateIsFinalizationDelayed(latestFinalizedSlot iotago.SlotIndex, latestCommitmentSlot iotago.SlotIndex) {
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

func (s *SyncManager) updateLastAcceptedBlock(lastAcceptedBlockID iotago.BlockID) (changed bool) {
	s.lastAcceptedBlockSlotLock.Lock()
	defer s.lastAcceptedBlockSlotLock.Unlock()

	if lastAcceptedBlockID.Slot() > s.lastAcceptedBlockSlot {
		s.lastAcceptedBlockSlot = lastAcceptedBlockID.Slot()

		return true
	}

	return false
}

func (s *SyncManager) LastAcceptedBlockSlot() iotago.SlotIndex {
	s.lastAcceptedBlockSlotLock.RLock()
	defer s.lastAcceptedBlockSlotLock.RUnlock()

	return s.lastAcceptedBlockSlot
}

func (s *SyncManager) updateLastConfirmedBlock(lastConfirmedBlockID iotago.BlockID) (changed bool) {
	s.lastConfirmedBlockSlotLock.Lock()
	defer s.lastConfirmedBlockSlotLock.Unlock()

	if lastConfirmedBlockID.Slot() > s.lastConfirmedBlockSlot {
		s.lastConfirmedBlockSlot = lastConfirmedBlockID.Slot()

		return true
	}

	return false
}

func (s *SyncManager) LastConfirmedBlockSlot() iotago.SlotIndex {
	s.lastConfirmedBlockSlotLock.RLock()
	defer s.lastConfirmedBlockSlotLock.RUnlock()

	return s.lastConfirmedBlockSlot
}

func (s *SyncManager) updateLatestCommitment(commitment *model.Commitment) (changed bool) {
	s.latestCommitmentLock.Lock()

	if s.latestCommitment != commitment {
		s.latestCommitment = commitment

		// we need to unlock the lock before calling updateIsFinalizationDelayed,
		// otherwise it might deadlock if isFinalizationDelayedLock is Rlocked in SyncStatus().
		s.latestCommitmentLock.Unlock()

		s.updateIsFinalizationDelayed(s.LatestFinalizedSlot(), commitment.Slot())

		return true
	}
	s.latestCommitmentLock.Unlock()

	return false
}

func (s *SyncManager) LatestCommitment() *model.Commitment {
	s.latestCommitmentLock.RLock()
	defer s.latestCommitmentLock.RUnlock()

	return s.latestCommitment
}

func (s *SyncManager) updateFinalizedSlot(slot iotago.SlotIndex) (changed bool) {
	s.latestFinalizedSlotLock.Lock()

	if s.latestFinalizedSlot != slot {
		s.latestFinalizedSlot = slot

		// we need to unlock the lock before calling updateIsFinalizationDelayed,
		// otherwise it might deadlock if isFinalizationDelayedLock is Rlocked in SyncStatus().
		s.latestFinalizedSlotLock.Unlock()

		s.updateIsFinalizationDelayed(slot, s.LatestCommitment().Slot())

		return true
	}
	s.latestFinalizedSlotLock.Unlock()

	return false
}

func (s *SyncManager) LatestFinalizedSlot() iotago.SlotIndex {
	s.latestFinalizedSlotLock.RLock()
	defer s.latestFinalizedSlotLock.RUnlock()

	return s.latestFinalizedSlot
}

func (s *SyncManager) updatePrunedEpoch(epoch iotago.EpochIndex, hasPruned bool) (changed bool) {
	s.lastPrunedEpochLock.Lock()
	defer s.lastPrunedEpochLock.Unlock()

	if s.lastPrunedEpoch != epoch || s.hasPruned != hasPruned {
		s.lastPrunedEpoch = epoch
		s.hasPruned = hasPruned

		return true
	}

	return false
}

func (s *SyncManager) LastPrunedEpoch() (iotago.EpochIndex, bool) {
	s.lastPrunedEpochLock.RLock()
	defer s.lastPrunedEpochLock.RUnlock()

	return s.lastPrunedEpoch, s.hasPruned
}
