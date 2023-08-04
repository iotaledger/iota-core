package rmc

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Manager struct {
	apiProvider api.Provider

	// accumulated work from accepted blocks per slot
	slotWork *shrinkingmap.ShrinkingMap[iotago.SlotIndex, iotago.WorkScore]

	//reference mana cost per slot
	rmc *shrinkingmap.ShrinkingMap[iotago.SlotIndex, iotago.Mana]

	// eviction parameters
	latestCommittedSlot iotago.SlotIndex

	// commitment loader
	commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error)

	mutex syncutils.RWMutex
}

func NewManager(apiProvider api.Provider, commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error)) *Manager {
	return &Manager{
		apiProvider:      apiProvider,
		slotWork:         shrinkingmap.New[iotago.SlotIndex, iotago.WorkScore](),
		rmc:              shrinkingmap.New[iotago.SlotIndex, iotago.Mana](),
		commitmentLoader: commitmentLoader,
	}
}

func (m *Manager) SetLatestCommittedSlot(index iotago.SlotIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.latestCommittedSlot = index
}

func (m *Manager) BlockAccepted(block *blocks.Block) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	blockID := block.ID()
	if blockID.Index() <= m.latestCommittedSlot {
		return ierrors.Errorf("cannot add block %s: slot with %d is already committed", blockID, blockID.Index())
	}

	slotWork, exists := m.slotWork.Get(blockID.Index())
	if !exists {
		slotWork = 0
	}
	if wasCreated := m.slotWork.Set(blockID.Index(), slotWork+block.WorkScore()); !wasCreated {
		return ierrors.Errorf("failed to add block to accepted blocks, blockID: %s", blockID)
	}

	return nil
}

func (m *Manager) CommitSlot(index iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// load the last RMC
	latestCommitment, err := m.commitmentLoader(index - 1)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to load commitment for slot %d", index-1)
	}
	lastRMC := latestCommitment.Commitment().RMC
	// load the slotWork for the current slot
	currentSlotWork, exists := m.slotWork.Get(index)
	if !exists {
		currentSlotWork = 0
	}
	// calculate the new RMC
	var newRMC iotago.Mana
	rmcParameters := m.apiProvider.APIForSlot(index).ProtocolParameters().RMCParameters()
	if currentSlotWork < rmcParameters.DecreaseThreshold {
		// TODO: use safemath here
		if lastRMC >= rmcParameters.RMCMin {
			newRMC = lastRMC - rmcParameters.Decrease
		}
	} else if currentSlotWork > rmcParameters.IncreaseThreshold {
		newRMC = lastRMC + rmcParameters.Increase
	} else {
		newRMC = lastRMC
	}
	// set the new RMC
	if wasCreated := m.rmc.Set(index, newRMC); !wasCreated {
		return 0, ierrors.Errorf("failed to set RMC for slot %d", index)
	}

	// evict slotWork for the current slot
	m.slotWork.Delete(index)

	// evict old RMC from current slot - maxCommittableAge
	maxCommittableAge := m.apiProvider.APIForSlot(index).ProtocolParameters().MaxCommittableAge()
	if index > maxCommittableAge {
		m.rmc.Delete(index - maxCommittableAge)
	}

	// update latestCommittedIndex
	if index <= m.latestCommittedSlot {
		return 0, ierrors.Errorf("cannot commit slot %d: already committed", index)
	}
	m.latestCommittedSlot = index

	return newRMC, nil
}

func (m *Manager) RMC(slot iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if slot > m.latestCommittedSlot {
		return 0, ierrors.Errorf("cannot get RMC for slot %d: not committed yet", slot)
	}
	// this should never happen when checking the RMC for a slot that is not committed yet

	if slot+m.apiProvider.CurrentAPI().ProtocolParameters().MaxCommittableAge() < m.latestCommittedSlot {
		return 0, ierrors.Errorf("cannot get RMC for slot %d: already evicted", slot)
	}

	rmc, exists := m.rmc.Get(slot)
	if !exists {
		// try to load the commitment
		latestCommitment, err := m.commitmentLoader(slot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get RMC for slot %d", slot)
		}
		rmc = latestCommitment.Commitment().RMC
		m.rmc.Set(slot, rmc)
	}

	return rmc, nil
}
