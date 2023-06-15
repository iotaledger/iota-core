package sybilprotection

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type MockSybilProtection struct {
	accounts *account.Accounts[iotago.AccountID, *iotago.AccountID]
	online   *advancedset.AdvancedSet[iotago.AccountID]
	aliases  *shrinkingmap.ShrinkingMap[string, iotago.AccountID]

	module.Module
}

func NewMockSybilProtection() *MockSybilProtection {
	return &MockSybilProtection{
		accounts: account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()),
		online:   advancedset.New[iotago.AccountID](),
		aliases:  shrinkingmap.New[string, iotago.AccountID](),
	}
}

func (m *MockSybilProtection) AddAccount(alias string, weight int64) iotago.AccountID {
	id := iotago.AccountID(tpkg.Rand32ByteArray())
	id.RegisterAlias(alias)
	m.accounts.Set(id, weight)
	m.aliases.Set(alias, id)

	return id
}

func (m *MockSybilProtection) AccountID(alias string) iotago.AccountID {
	id, exists := m.aliases.Get(alias)
	if !exists {
		panic(fmt.Sprintf("alias %s does not exist", alias))
	}

	return id
}

func (m *MockSybilProtection) SetOnline(aliases ...string) {
	for _, alias := range aliases {
		m.online.Add(m.AccountID(alias))
	}
}

func (m *MockSybilProtection) SetOffline(aliases ...string) {
	for _, alias := range aliases {
		m.online.Delete(m.AccountID(alias))
	}
}

func (m *MockSybilProtection) Accounts() *account.Accounts[iotago.AccountID, *iotago.AccountID] {
	return m.accounts
}

func (m *MockSybilProtection) Committee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID] {
	return m.accounts.SelectAccounts(lo.Keys(lo.PanicOnErr(m.accounts.Map()))...)
}

func (m *MockSybilProtection) OnlineCommittee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID] {
	return m.accounts.SelectAccounts(m.online.Slice()...)
}

func (m *MockSybilProtection) Shutdown() {}
