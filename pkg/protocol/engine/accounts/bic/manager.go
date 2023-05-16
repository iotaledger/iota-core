package bic

import (
	"fmt"
	"sync"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BIC is a Block Issuer Credits module responsible for tracking account-based mana balances..
type BIC struct {
	workers *workerpool.Group
	// balances represents the Block Issuer Credits of all registered accounts, isupdated on the slot commitment.
	balances *account.Accounts[iotago.AccountID, *iotago.AccountID]

	ledger ledger.Ledger

	mutex sync.RWMutex

	module.Module
}

func NewProvider(opts ...options.Option[BIC]) module.Provider[*engine.Engine, accounts.BlockIssuanceCredits] {
	return module.Provide(func(e *engine.Engine) accounts.BlockIssuanceCredits {
		return options.Apply(
			&BIC{
				workers:  e.Workers.CreateGroup("BIC"),
				balances: account.NewAccounts[iotago.AccountID](e.Storage.Accounts(StoreKeyPrefixBIC)),
			},
			opts, func(b *BIC) {
				e.HookConstructed(func() {
					//	e.Events.TransactionAccepted.Attach(events.NewClosure(func(tx *ledgerstate.Transaction) {
					//		b.workers.Submit(func() {
					//			b.mutex.Lock()
					//			defer b.mutex.Unlock()
					//
					//			// TODO update mana
					//		})
					//	}))
				})
			})
	})
}

func (b *BIC) CommitSlot(slotDiff *ledgerstate.SlotDiff) (bicRoot iotago.Identifier, err error) {
	// TODO do we need to store the index, if yes should it be in the engine store or should we create new kv store as in the ledger?
	bicIndex, err := b.ReadBICIndex()
	if err != nil {
		return iotago.Identifier{}, err
	}
	if slotDiff.Index != bicIndex+1 {
		panic(fmt.Errorf("there is a gap in the bicstate %d vs %d", bicIndex, slotDiff.Index))
	}

	b.ApplyDiff(slotDiff)

	return iotago.Identifier{}, nil
}

func (b *BIC) BIC() *account.Accounts[iotago.AccountID, *iotago.AccountID] {
	return b.balances
}

func (b *BIC) AccountBIC(id iotago.AccountID) (account *accounts.Account, err error) {
	return nil, nil
}

func (b *BIC) Shutdown() {
}

func (b *BIC) ReadBICIndex() (index iotago.SlotIndex, err error) {
	return 0, nil
}

func (b *BIC) ApplyDiff(slotDiff *ledgerstate.SlotDiff) {
	// todo handle locking

}
