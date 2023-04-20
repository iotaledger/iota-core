package slotnotarization

import (
	"io"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	defaultMinSlotCommittableAge = 6
)

// region Manager //////////////////////////////////////////////////////////////////////////////////////////////////////

// Manager is the component that manages the slot commitments.
type Manager struct {
	events        *notarization.Events
	slotMutations *SlotMutations
	attestations  *Attestations

	storage         *storage.Storage
	commitmentMutex sync.RWMutex

	acceptedTimeFunc func() time.Time

	slotTimeProviderFunc func() *iotago.SlotTimeProvider

	optsMinCommittableSlotAge iotago.SlotIndex

	module.Module
}

func NewProvider(opts ...options.Option[Manager]) module.Provider[*engine.Engine, notarization.Notarization] {
	return module.Provide(func(e *engine.Engine) notarization.Notarization {
		return options.Apply(&Manager{
			events:                    notarization.NewEvents(),
			optsMinCommittableSlotAge: defaultMinSlotCommittableAge,
		}, opts,
			func(m *Manager) {
				m.slotTimeProviderFunc = func() *iotago.SlotTimeProvider {
					return e.API().SlotTimeProvider()
				}

				m.attestations = NewAttestations(e.Storage.Permanent.Attestations,
					e.Storage.Prunable.Attestations,
					func() *account.Accounts {
						// Using a func here because at this point SybilProtection is not guaranteed to exist since the engine has not been constructed, but other modules already might want to use `Attestations()`
						return e.SybilProtection.Accounts()
					}, m.slotTimeProviderFunc)

				m.HookInitialized(m.attestations.TriggerInitialized)

				e.HookConstructed(func() {
					m.storage = e.Storage
					m.acceptedTimeFunc = e.Clock.Accepted().Time

					wpBlocks := e.Workers.CreatePool("NotarizationManager.Blocks", 1)           // Using just 1 worker to avoid contention
					wpCommitments := e.Workers.CreatePool("NotarizationManager.Commitments", 1) // Using just 1 worker to avoid contention

					e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
						if err := m.NotarizeAcceptedBlock(block); err != nil {
							e.Events.Error.Trigger(errors.Wrapf(err, "failed to add accepted block %s to slot", block.ID()))
						}
					}, event.WithWorkerPool(wpBlocks))

					// Slots are committed whenever ATT advances, start committing only when bootstrapped.
					e.Events.Clock.AcceptedTimeUpdated.Hook(m.TryCommitUntil, event.WithWorkerPool(wpCommitments))

					m.slotMutations = NewSlotMutations(e.SybilProtection.Accounts(), e.Storage.Settings().LatestCommitment().Index)

					m.events.AcceptedBlockRemoved.LinkTo(m.slotMutations.AcceptedBlockRemoved)
					e.Events.Notarization.LinkTo(m.events)

					m.TriggerInitialized()
				})
			},
			(*Manager).TriggerConstructed)
	})
}

func (m *Manager) Attestations() notarization.Attestations {
	return m.attestations
}

func (m *Manager) Shutdown() {
	m.TriggerStopped()
}

// TryCommitUntil tries to create slot commitments until the new provided acceptance time.
func (m *Manager) TryCommitUntil(acceptanceTime time.Time) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	if index := m.slotTimeProviderFunc().IndexFromTime(acceptanceTime); index > m.storage.Settings().LatestCommitment().Index {
		m.tryCommitSlotUntil(index)
	}
}

// IsBootstrapped returns if the Manager finished committing all pending slots up to the current acceptance time.
func (m *Manager) IsBootstrapped() bool {
	// If acceptance time is in slot 10, then the latest committable index is 3 (with optsMinCommittableSlotAge=6), because there are 6 full slots between slot 10 and slot 3.
	// All slots smaller than 4 are committable, so in order to check if slot 3 is committed it's necessary to do m.optsMinCommittableSlotAge-1,
	// otherwise we'd expect slot 4 to be committed in order to be fully committed, which is impossible.
	return m.storage.Settings().LatestCommitment().Index >= m.slotTimeProviderFunc().IndexFromTime(m.acceptedTimeFunc())-m.optsMinCommittableSlotAge-1
}

func (m *Manager) NotarizeAcceptedBlock(block *blocks.Block) (err error) {
	if err = m.slotMutations.AddAcceptedBlock(block); err != nil {
		return errors.Wrap(err, "failed to add accepted block to slot mutations")
	}

	if _, err = m.attestations.Add(iotago.NewAttestation(block.Block(), m.slotTimeProviderFunc())); err != nil {
		return errors.Wrap(err, "failed to add block to attestations")
	}

	return
}

func (m *Manager) Import(reader io.ReadSeeker) (err error) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()

	if err = m.attestations.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import attestations")
	}

	m.TriggerInitialized()

	return
}

func (m *Manager) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	m.commitmentMutex.RLock()
	defer m.commitmentMutex.RUnlock()

	if err = m.attestations.Export(writer, targetSlot); err != nil {
		return errors.Wrap(err, "failed to export attestations")
	}

	return
}

// MinCommittableSlotAge returns the minimum age of a slot to be committable.
func (m *Manager) MinCommittableSlotAge() iotago.SlotIndex {
	return m.optsMinCommittableSlotAge
}

func (m *Manager) tryCommitSlotUntil(acceptedBlockIndex iotago.SlotIndex) {
	for i := m.storage.Settings().LatestCommitment().Index + 1; i <= acceptedBlockIndex; i++ {
		if !m.isCommittable(i, acceptedBlockIndex) {
			return
		}

		if !m.createCommitment(i) {
			return
		}
	}
}

func (m *Manager) isCommittable(index, acceptedBlockIndex iotago.SlotIndex) bool {
	return index < acceptedBlockIndex-m.optsMinCommittableSlotAge
}

func (m *Manager) createCommitment(index iotago.SlotIndex) (success bool) {
	latestCommitment := m.storage.Settings().LatestCommitment()
	if index != latestCommitment.Index+1 {
		m.events.Error.Trigger(errors.Errorf("cannot create commitment for slot %d, latest commitment is for slot %d", index, latestCommitment.Index))

		return false
	}

	acceptedBlocks, err := m.slotMutations.Evict(index)
	if err != nil {
		m.events.Error.Trigger(errors.Wrap(err, "failed to commit mutations"))
		return false
	}

	attestations, attestationsWeight, err := m.attestations.Commit(index)
	if err != nil {
		m.events.Error.Trigger(errors.Wrap(err, "failed to commit attestations"))
		return false
	}

	newCommitment := iotago.NewCommitment(
		index,
		latestCommitment.MustID(),
		iotago.NewRoots(
			iotago.Identifier(acceptedBlocks.Root()),
			iotago.Identifier{},
			iotago.Identifier(attestations.Root()),
			iotago.Identifier{},
			iotago.Identifier(m.slotMutations.weights.Root()),
		).ID(),
		m.storage.Settings().LatestCommitment().CumulativeWeight+attestationsWeight,
	)

	if err = m.storage.Settings().SetLatestCommitment(newCommitment); err != nil {
		m.events.Error.Trigger(errors.Wrap(err, "failed to set latest commitment"))
		return false
	}

	if err = m.storage.Commitments().Store(newCommitment); err != nil {
		m.events.Error.Trigger(errors.Wrap(err, "failed to store latest commitment"))
		return false
	}

	m.events.SlotCommitted.Trigger(&notarization.SlotCommittedDetails{
		Commitment:            newCommitment,
		AcceptedBlocks:        acceptedBlocks,
		ActiveValidatorsCount: 0,
	})

	return true
}

func (m *Manager) PerformLocked(perform func(m notarization.Notarization)) {
	m.commitmentMutex.Lock()
	defer m.commitmentMutex.Unlock()
	perform(m)
}

var _ notarization.Notarization = new(Manager)

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Options //////////////////////////////////////////////////////////////////////////////////////////////////////

// WithMinCommittableSlotAge specifies how old a slot has to be for it to be committable.
func WithMinCommittableSlotAge(age iotago.SlotIndex) options.Option[Manager] {
	return func(manager *Manager) {
		manager.optsMinCommittableSlotAge = age
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
