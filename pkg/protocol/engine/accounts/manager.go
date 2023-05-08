package accounts

import (
	"sync"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	PrefixWeights byte = iota
	PrefixMana
)

// TODO rename it as it consists ot only BIC...

// BIC is a Block Issuer Credits module responsible for tracking account-based mana balances..
type BIC struct {
	workers *workerpool.Group
	// balances represents the Block Issuer Credits of all registered accounts, isupdated on the slot commitment.
	balances *account.Accounts[iotago.AccountID, *iotago.AccountID]
	// mana represents the sum of stored and potential mana of all registered accounts, is updated on tx acceptance..
	mana *account.Accounts[iotago.AccountID, *iotago.AccountID]

	mutex sync.RWMutex

	module.Module
}

func NewProvider(opts ...options.Option[BIC]) module.Provider[*engine.Engine, Weights] {
	return module.Provide(func(e *engine.Engine) Weights {
		return options.Apply(
			&BIC{
				workers:  e.Workers.CreateGroup("BIC"),
				balances: account.NewAccounts[iotago.AccountID](e.Storage.Accounts(PrefixWeights)),
				mana:     account.NewAccounts[iotago.AccountID](e.Storage.Accounts(PrefixMana)),
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

func (b *BIC) BIC() *account.Accounts[iotago.AccountID, *iotago.AccountID] {
	return b.balances
}

func (b *BIC) AccountBIC(id iotago.AccountID) (account *iotago.AccountID, err error) {
	return nil, nil
}

// Mana is the stored and potential ana vale of an account collected on the UTXO layer - used by the Scheduler.
func (b *BIC) Mana(id iotago.AccountID) (mana *Mana, err error) {
	return nil, nil
}

func (b *BIC) Shutdown() {
}
