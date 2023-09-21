package rmc

import (
	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Manager struct {
	apiProvider iotago.APIProvider

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

func NewManager(apiProvider iotago.APIProvider, commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error)) *Manager {
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

	m.slotWork.Compute(blockID.Index(), func(currentValue iotago.WorkScore, _ bool) iotago.WorkScore {
		return currentValue + block.WorkScore()
	})

	return nil
}

func (m *Manager) CommitSlot(index iotago.SlotIndex) (iotago.Mana, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// check if the slot is already committed
	if index <= m.latestCommittedSlot {
		return 0, ierrors.Errorf("cannot commit slot %d: already committed", index)
	}

	// load the last RMC
	latestCommitment, err := m.commitmentLoader(index - 1)
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to load commitment for slot %d", index-1)
	}
	lastRMC := latestCommitment.Commitment().ReferenceManaCost
	// load the slotWork for the current slot
	currentSlotWork, exists := m.slotWork.Get(index)
	if !exists {
		currentSlotWork = 0
	}
	// calculate the new RMC
	var newRMC iotago.Mana
	ccParameters := m.apiProvider.APIForSlot(index).ProtocolParameters().CongestionControlParameters()
	if currentSlotWork < ccParameters.DecreaseThreshold && lastRMC > ccParameters.MinReferenceManaCost {
		newRMC, err = safemath.SafeSub(lastRMC, ccParameters.Decrease)
		if err == nil {
			newRMC = lo.Max(newRMC, ccParameters.MinReferenceManaCost)
		}
	} else if currentSlotWork > ccParameters.IncreaseThreshold {
		newRMC, err = safemath.SafeAdd(lastRMC, ccParameters.Increase)
	} else {
		newRMC = lastRMC
	}
	if err != nil {
		return 0, ierrors.Wrapf(err, "failed to calculate RMC for slot %d", index)
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

	if slot+m.apiProvider.APIForSlot(slot).ProtocolParameters().MaxCommittableAge() < m.latestCommittedSlot {
		return 0, ierrors.Errorf("cannot get RMC for slot %d: already evicted", slot)
	}

	rmc, exists := m.rmc.Get(slot)
	if !exists {
		// try to load the commitment
		// this should only be required when starting from a snapshot as we do not include RMC in snapshots
		latestCommitment, err := m.commitmentLoader(slot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get RMC for slot %d", slot)
		}
		rmc = latestCommitment.Commitment().ReferenceManaCost
	}

	return rmc, nil
}
