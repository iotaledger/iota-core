package slotnotarization

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Manager is the component that manages the slot commitments.
type Manager struct {
	events        *notarization.Events
	slotMutations *SlotMutations

	workers                      *workerpool.Group
	errorHandler                 func(error)
	acceptedBlockProcessedDetach func()

	attestation         attestation.Attestations
	ledger              ledger.Ledger
	sybilProtection     sybilprotection.SybilProtection
	upgradeOrchestrator upgrade.Orchestrator

	storage *storage.Storage

	acceptedTimeFunc  func() time.Time
	minCommittableAge iotago.SlotIndex
	apiProvider       iotago.APIProvider

	commitmentMutex syncutils.Mutex

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, notarization.Notarization] {
	return module.Provide(func(e *engine.Engine) notarization.Notarization {
		m := NewManager(e.CurrentAPI().ProtocolParameters().MinCommittableAge(), e.Workers.CreateGroup("NotarizationManager"), e.ErrorHandler("notarization"))

		m.apiProvider = e

		e.HookConstructed(func() {
			m.storage = e.Storage
			m.acceptedTimeFunc = e.Clock.Accepted().Time

			m.ledger = e.Ledger
			m.sybilProtection = e.SybilProtection
			m.attestation = e.Attestations
			m.upgradeOrchestrator = e.UpgradeOrchestrator

			wpBlocks := m.workers.CreatePool("Blocks", workerpool.WithWorkerCount(1)) // Using just 1 worker to avoid contention

			m.acceptedBlockProcessedDetach = e.Events.AcceptedBlockProcessed.Hook(func(block *blocks.Block) {
				if err := m.notarizeAcceptedBlock(block); err != nil {
					m.errorHandler(ierrors.Wrapf(err, "failed to add accepted block %s to slot", block.ID()))
				}
				m.tryCommitUntil(block.ID().Slot())

				block.SetNotarized()
			}, event.WithWorkerPool(wpBlocks)).Unhook

			e.Events.Notarization.LinkTo(m.events)

			m.slotMutations = NewSlotMutations(e.Storage.Settings().LatestCommitment().Slot())

			m.TriggerConstructed()

			e.LatestCachedSlot.OnUpdate(func(oldValue, newValue iotago.SlotIndex) {
				if newValue < oldValue {
					m.slotMutations.ClearCache(newValue+1, oldValue)
				}
			})

			m.TriggerInitialized()
		})

		return m
	})
}

func NewManager(minCommittableAge iotago.SlotIndex, workers *workerpool.Group, errorHandler func(error)) *Manager {
	return &Manager{
		minCommittableAge: minCommittableAge,
		events:            notarization.NewEvents(),
		workers:           workers,
		errorHandler:      errorHandler,
	}
}

func (m *Manager) Shutdown() {
	m.TriggerStopped()
	// Alternative 2
	if m.acceptedBlockProcessedDetach != nil {
		m.acceptedBlockProcessedDetach()
	}
	m.workers.Shutdown()
}

// tryCommitUntil tries to create slot commitments until the new provided acceptance time.
func (m *Manager) tryCommitUntil(commitUntilSlot iotago.SlotIndex) {
	if slot := commitUntilSlot; slot > m.storage.Settings().LatestCommitment().Slot() {
		m.tryCommitSlotUntil(slot)
	}
}

func (m *Manager) ForceCommit(slot iotago.SlotIndex) (*model.Commitment, error) {
	if m.WasStopped() {
		return nil, ierrors.New("notarization manager was stopped")
	}

	commitment, err := m.createCommitment(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to create commitment for slot %d", slot)
	}

	return commitment, nil
}

func (m *Manager) ForceCommitUntil(commitUntilSlot iotago.SlotIndex) error {
	for i := m.storage.Settings().LatestCommitment().Slot() + 1; i <= commitUntilSlot; i++ {
		if _, err := m.ForceCommit(i); err != nil {
			return ierrors.Wrapf(err, "failed to force commit slot %d", i)
		}
	}

	return nil
}

// IsBootstrapped returns if the Manager finished committing all pending slots up to the current acceptance time.
func (m *Manager) IsBootstrapped() bool {
	// If acceptance time is somewhere in the middle of slot 10, then the latest committable index is 4 (with minCommittableAge=6),
	// because there are 5 full slots and 1 that is still not finished between slot 10 and slot 4.
	// All slots smaller or equal to 4 are committable.
	latestIndex := m.storage.Settings().LatestCommitment().Slot()
	return latestIndex+m.minCommittableAge >= m.apiProvider.APIForSlot(latestIndex).TimeProvider().SlotFromTime(m.acceptedTimeFunc())
}

func (m *Manager) notarizeAcceptedBlock(block *blocks.Block) (err error) {
	if err = m.slotMutations.AddAcceptedBlock(block); err != nil {
		return ierrors.Wrap(err, "failed to add accepted block to slot mutations")
	}

	m.attestation.AddAttestationFromValidationBlock(block)

	return
}

func (m *Manager) tryCommitSlotUntil(acceptedBlockIndex iotago.SlotIndex) {
	for i := m.storage.Settings().LatestCommitment().Slot() + 1; i <= acceptedBlockIndex; i++ {
		if m.WasStopped() {
			break
		}

		if !m.isCommittable(i, acceptedBlockIndex) {
			return
		}

		if _, err := m.createCommitment(i); err != nil {
			m.errorHandler(ierrors.Wrapf(err, "failed to create commitment for slot %d", i))
			return
		}
	}
}

func (m *Manager) isCommittable(index, acceptedBlockIndex iotago.SlotIndex) bool {
	return index+m.minCommittableAge <= acceptedBlockIndex
}

func (m *Manager) createCommitment(slot iotago.SlotIndex) (*model.Commitment, error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	latestCommitment := m.storage.Settings().LatestCommitment()
	if slot != latestCommitment.Slot()+1 {
		return nil, ierrors.Errorf("cannot create commitment for slot %d, latest commitment is for slot %d", slot, latestCommitment.Slot())
	}

	// Set createIfMissing to true to make sure that this is never nil. Will get evicted later on anyway.
	acceptedBlocks := m.slotMutations.AcceptedBlocks(slot, true)
	if err := acceptedBlocks.Commit(); err != nil {
		return nil, ierrors.Wrap(err, "failed to commit accepted blocks")
	}

	cumulativeWeight, attestationsRoot, err := m.attestation.Commit(slot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to commit attestations")
	}

	stateRoot, mutationRoot, accountRoot, err := m.ledger.CommitSlot(slot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to commit ledger")
	}

	committeeRoot, rewardsRoot, err := m.sybilProtection.CommitSlot(slot)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to commit sybil protection")
	}
	apiForSlot := m.apiProvider.APIForSlot(slot)

	protocolParametersAndVersionsHash, err := m.upgradeOrchestrator.Commit(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to commit protocol parameters and versions in upgrade orchestrator for slot %d", slot)
	}

	roots := iotago.NewRoots(
		iotago.Identifier(acceptedBlocks.Root()),
		mutationRoot,
		attestationsRoot,
		stateRoot,
		accountRoot,
		committeeRoot,
		rewardsRoot,
		protocolParametersAndVersionsHash,
	)

	// calculate the new RMC
	rmc, err := m.ledger.RMCManager().CommitSlot(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to commit RMC for slot %d", slot)
	}

	newCommitment := iotago.NewCommitment(
		apiForSlot.ProtocolParameters().Version(),
		slot,
		latestCommitment.ID(),
		roots.ID(),
		cumulativeWeight,
		rmc,
	)

	newModelCommitment, err := model.CommitmentFromCommitment(newCommitment, apiForSlot, serix.WithValidation())
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to create model commitment for commitment %s", newCommitment.MustID())
	}

	rootsStorage, err := m.storage.Roots(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed get roots storage for commitment %s", newModelCommitment.ID())
	}
	if err = rootsStorage.Store(newModelCommitment.ID(), roots); err != nil {
		return nil, ierrors.Wrapf(err, "failed to store latest roots for commitment %s", newModelCommitment.ID())
	}

	if err = m.storage.Commitments().Store(newModelCommitment); err != nil {
		return nil, ierrors.Wrapf(err, "failed to store latest commitment %s", newModelCommitment.ID())
	}

	m.events.SlotCommitted.Trigger(&notarization.SlotCommittedDetails{
		Commitment:            newModelCommitment,
		AcceptedBlocks:        acceptedBlocks,
		ActiveValidatorsCount: 0,
	})

	if err = m.storage.Settings().SetLatestCommitment(newModelCommitment); err != nil {
		return nil, ierrors.Wrap(err, "failed to set latest commitment")
	}

	m.events.LatestCommitmentUpdated.Trigger(newModelCommitment)

	if err = m.slotMutations.Evict(slot); err != nil {
		m.errorHandler(ierrors.Wrapf(err, "failed to evict slotMutations at slot: %d", slot))
	}

	return newModelCommitment, nil
}

func (m *Manager) AcceptedBlocksCount(index iotago.SlotIndex) int {
	return m.slotMutations.AcceptedBlocksCount(index)
}

var _ notarization.Notarization = new(Manager)
