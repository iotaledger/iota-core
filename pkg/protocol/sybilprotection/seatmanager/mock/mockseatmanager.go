package mock

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type ManualPOA struct {
	events         *seatmanager.Events
	apiProvider    iotago.APIProvider
	committeeStore epochstore.Store[*account.SeatedAccounts]

	accounts  *account.Accounts
	committee *account.SeatedAccounts
	online    ds.Set[account.SeatIndex]
	aliases   *shrinkingmap.ShrinkingMap[string, iotago.AccountID]

	module.Module
}

func NewManualPOA(e iotago.APIProvider, committeeStore epochstore.Store[*account.SeatedAccounts]) *ManualPOA {
	m := &ManualPOA{
		events:         seatmanager.NewEvents(),
		apiProvider:    e,
		committeeStore: committeeStore,
		accounts:       account.NewAccounts(),
		online:         ds.NewSet[account.SeatIndex](),
		aliases:        shrinkingmap.New[string, iotago.AccountID](),
	}
	m.committee = m.accounts.SeatedAccounts()

	return m
}

func NewManualPOAProvider() module.Provider[*engine.Engine, seatmanager.SeatManager] {
	return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
		poa := NewManualPOA(e, e.Storage.Committee())
		e.Events.PostSolidFilter.BlockAllowed.Hook(func(block *blocks.Block) {
			poa.events.BlockProcessed.Trigger(block)
		})

		e.Events.SeatManager.LinkTo(poa.events)

		return poa
	})
}

func (m *ManualPOA) AddRandomAccounts(aliases ...string) (accountIDs []iotago.AccountID) {
	accountIDs = make([]iotago.AccountID, len(aliases))

	for i, alias := range aliases {
		id := iotago.AccountID(tpkg.Rand32ByteArray())
		id.RegisterAlias(alias)
		if err := m.accounts.Set(id, &account.Pool{ // We don't care about pools with PoA, but need to set something to avoid division by zero errors.
			PoolStake:      1,
			ValidatorStake: 1,
			FixedCost:      1,
		}); err != nil {
			panic(err)
		}

		m.aliases.Set(alias, id)

		accountIDs[i] = id
	}

	m.committee = m.accounts.SeatedAccounts()

	if err := m.committeeStore.Store(0, m.committee); err != nil {
		panic(err)
	}

	return accountIDs
}

func (m *ManualPOA) AddAccount(id iotago.AccountID, alias string) iotago.AccountID {
	if err := m.accounts.Set(id, &account.Pool{ // We don't care about pools with PoA, but need to set something to avoid division by zero errors.
		PoolStake:      1,
		ValidatorStake: 1,
		FixedCost:      1,
	}); err != nil {
		panic(err)
	}
	m.aliases.Set(alias, id)

	m.committee = m.accounts.SeatedAccounts()

	if err := m.committeeStore.Store(0, m.committee); err != nil {
		panic(err)
	}

	return id
}

func (m *ManualPOA) AccountID(alias string) iotago.AccountID {
	id, exists := m.aliases.Get(alias)
	if !exists {
		panic(fmt.Sprintf("alias %s does not exist", alias))
	}

	return id
}

func (m *ManualPOA) GetSeat(alias string) (account.SeatIndex, bool) {
	return m.committee.GetSeat(m.AccountID(alias))
}

func (m *ManualPOA) SetOnline(aliases ...string) {
	for _, alias := range aliases {
		seat, exists := m.committee.GetSeat(m.AccountID(alias))
		if !exists {
			panic(fmt.Sprintf("alias %s does not exist", alias))
		}
		m.online.Add(seat)
	}
}

func (m *ManualPOA) SetOffline(aliases ...string) {
	for _, alias := range aliases {
		seat, exists := m.committee.GetSeat(m.AccountID(alias))
		if !exists {
			panic(fmt.Sprintf("alias %s does not exist", alias))
		}
		m.online.Delete(seat)
	}
}

func (m *ManualPOA) Accounts() *account.Accounts {
	return m.accounts
}

// CommitteeInSlot returns the set of validators selected to be part of the committee in the given slot.
func (m *ManualPOA) CommitteeInSlot(slot iotago.SlotIndex) (*account.SeatedAccounts, bool) {
	return m.committeeInEpoch(m.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot))
}

// CommitteeInEpoch returns the set of validators selected to be part of the committee in the given epoch.
func (m *ManualPOA) CommitteeInEpoch(epoch iotago.EpochIndex) (*account.SeatedAccounts, bool) {
	return m.committeeInEpoch(epoch)
}

func (m *ManualPOA) committeeInEpoch(epoch iotago.EpochIndex) (*account.SeatedAccounts, bool) {
	c, err := m.committeeStore.Load(epoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	if c == nil {
		return nil, false
	}

	return c, true
}

func (m *ManualPOA) OnlineCommittee() ds.Set[account.SeatIndex] {
	return m.online
}

func (m *ManualPOA) SeatCountInSlot(_ iotago.SlotIndex) int {
	return m.committee.SeatCount()
}
func (m *ManualPOA) SeatCountInEpoch(_ iotago.EpochIndex) int {
	return m.committee.SeatCount()
}

func (m *ManualPOA) RotateCommittee(epoch iotago.EpochIndex, validators accounts.AccountsData) (*account.SeatedAccounts, error) {
	if m.committee == nil || m.accounts.Size() == 0 {
		m.accounts = account.NewAccounts()

		for _, validatorData := range validators {
			if err := m.accounts.Set(validatorData.ID, &account.Pool{
				PoolStake:      validatorData.ValidatorStake + validatorData.DelegationStake,
				ValidatorStake: validatorData.ValidatorStake,
				FixedCost:      validatorData.FixedCost,
			}); err != nil {
				return nil, ierrors.Wrapf(err, "error while setting pool for epoch %d for validator %s", epoch, validatorData.ID.String())
			}
		}
		m.committee = m.accounts.SeatedAccounts()
	}

	if err := m.committeeStore.Store(epoch, m.committee); err != nil {
		panic(err)
	}

	return m.committee, nil
}

func (m *ManualPOA) ReuseCommittee(currentEpoch iotago.EpochIndex, targetEpoch iotago.EpochIndex) (*account.SeatedAccounts, error) {
	currentCommittee, exists := m.committeeInEpoch(currentEpoch)
	if !exists {
		// that should never happen as it is already the fallback strategy
		panic(fmt.Sprintf("committee for current epoch %d not found", currentEpoch))
	}

	if currentCommittee.SeatCount() == 0 {
		return nil, ierrors.New("committee must not be empty")
	}

	committee, err := currentCommittee.Reuse()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to reuse committee from epoch %d", currentEpoch)
	}

	if m.committee == nil || m.accounts.Size() == 0 {
		committeeAccounts, err := committee.Accounts()
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to set manual PoA committee for epoch %d", targetEpoch)
		}

		m.accounts = committeeAccounts
		m.committee = committee
	}

	if err := m.committeeStore.Store(targetEpoch, committee); err != nil {
		panic(err)
	}

	return committee, nil
}

func (m *ManualPOA) InitializeCommittee(_ iotago.EpochIndex, _ time.Time) error {
	return nil
}

func (m *ManualPOA) Shutdown() {}

var _ seatmanager.SeatManager = &ManualPOA{}
