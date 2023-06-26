package mock

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type ManualPOA struct {
	accounts  *account.Accounts
	committee *account.SeatedAccounts
	online    *advancedset.AdvancedSet[account.SeatIndex]
	aliases   *shrinkingmap.ShrinkingMap[string, iotago.AccountID]

	module.Module
}

func NewManualPOA() *ManualPOA {
	m := &ManualPOA{
		accounts: account.NewAccounts(),
		online:   advancedset.New[account.SeatIndex](),
		aliases:  shrinkingmap.New[string, iotago.AccountID](),
	}
	m.committee = m.accounts.SelectCommittee()

	return m
}

func (m *ManualPOA) AddAccount(alias string) iotago.AccountID {
	id := iotago.AccountID(tpkg.Rand32ByteArray())
	id.RegisterAlias(alias)
	m.accounts.Set(id, &account.Pool{}) // We don't care about pools with PoA
	m.aliases.Set(alias, id)
	m.committee.Set(account.SeatIndex(m.committee.SeatCount()), id)

	return id
}

func (m *ManualPOA) AccountID(alias string) iotago.AccountID {
	id, exists := m.aliases.Get(alias)
	if !exists {
		panic(fmt.Sprintf("alias %s does not exist", alias))
	}

	return id
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

func (m *ManualPOA) Committee(_ iotago.SlotIndex) *account.SeatedAccounts {
	return m.committee
}

func (m *ManualPOA) OnlineCommittee() *advancedset.AdvancedSet[account.SeatIndex] {
	return m.online
}

func (m *ManualPOA) SeatCount() int {
	return m.committee.SeatCount()
}

func (m *ManualPOA) RotateCommittee(_ iotago.EpochIndex, _ *account.Accounts) *account.SeatedAccounts {
	return m.committee
}

func (m *ManualPOA) Shutdown() {}

var _ sybilprotection.SybilProtection = &ManualPOA{}
