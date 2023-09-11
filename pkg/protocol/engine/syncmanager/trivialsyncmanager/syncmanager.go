package trivialsyncmanager

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
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

	isBootstrapped     bool
	isBootstrappedLock syncutils.RWMutex

	optsIsBootstrappedFunc    isBootstrappedFunc
	optsBootstrappedThreshold time.Duration

	module.Module
}

// NewProvider creates a new SyncManager provider.
func NewProvider(opts ...options.Option[SyncManager]) module.Provider[*engine.Engine, syncmanager.SyncManager] {
	return module.Provide(func(e *engine.Engine) syncmanager.SyncManager {
		s := New(e, e.Storage.Settings().LatestCommitment(), e.Storage.Settings().LatestFinalizedSlot(), opts...)
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

		e.Events.StoragePruned.Hook(func(index iotago.EpochIndex) {
			if s.updatePrunedEpoch(index, true) {
				s.triggerUpdate()
			}
		}, asyncOpt)

		e.Events.SyncManager.LinkTo(s.events)
		s.TriggerInitialized()

		return s
	})
}

func New(e *engine.Engine, latestCommitment *model.Commitment, finalizedSlot iotago.SlotIndex, opts ...options.Option[SyncManager]) *SyncManager {
	return options.Apply(&SyncManager{
		events:                 syncmanager.NewEvents(),
		engine:                 e,
		syncThreshold:          10 * time.Second,
		lastAcceptedBlockSlot:  latestCommitment.Index(),
		lastConfirmedBlockSlot: latestCommitment.Index(),
		latestCommitment:       latestCommitment,
		latestFinalizedSlot:    finalizedSlot,

		optsBootstrappedThreshold: 10 * time.Second,
	}, opts, func(s *SyncManager) {
		s.updatePrunedEpoch(s.engine.Storage.LastPrunedEpoch())

		// set the default bootstrapped function
		if s.optsIsBootstrappedFunc == nil {
			s.optsIsBootstrappedFunc = func(e *engine.Engine) bool {
				return time.Since(e.Clock.Accepted().RelativeTime()) < s.optsBootstrappedThreshold && e.Notarization.IsBootstrapped()
			}
		}
	})
}

func (s *SyncManager) SyncStatus() *syncmanager.SyncStatus {
	s.lastAcceptedBlockSlotLock.RLock()
	s.lastConfirmedBlockSlotLock.RLock()
	s.latestCommitmentLock.RLock()
	s.latestFinalizedSlotLock.RLock()
	s.lastPrunedEpochLock.RLock()
	defer s.lastAcceptedBlockSlotLock.RUnlock()
	defer s.lastConfirmedBlockSlotLock.RUnlock()
	defer s.latestCommitmentLock.RUnlock()
	defer s.latestFinalizedSlotLock.RUnlock()
	defer s.lastPrunedEpochLock.RUnlock()

	return &syncmanager.SyncStatus{
		NodeSynced:             s.IsNodeSynced(),
		LastAcceptedBlockSlot:  s.lastAcceptedBlockSlot,
		LastConfirmedBlockSlot: s.lastConfirmedBlockSlot,
		LatestCommitment:       s.latestCommitment,
		LatestFinalizedSlot:    s.latestFinalizedSlot,
		LastPrunedEpoch:        s.lastPrunedEpoch,
		HasPruned:              s.hasPruned,
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

func (s *SyncManager) updateFinalizedSlot(index iotago.SlotIndex) (changed bool) {
	s.latestFinalizedSlotLock.Lock()
	defer s.latestFinalizedSlotLock.Unlock()

	if s.latestFinalizedSlot != index {
		s.latestFinalizedSlot = index
		return true
	}

	return false
}

func (s *SyncManager) updatePrunedEpoch(index iotago.EpochIndex, hasPruned bool) (changed bool) {
	s.lastPrunedEpochLock.Lock()
	defer s.lastPrunedEpochLock.Unlock()

	if s.lastPrunedEpoch != index {
		s.lastPrunedEpoch = index
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
