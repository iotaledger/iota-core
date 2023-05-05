package accounts

import (
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Weights is the minimal interface for the Accounts component of the IOTA protocol.
type Weights interface {
	// BIC returns Block Issuer Credits of all registered accounts.
	BIC() *account.Accounts[iotago.AccountID, *iotago.AccountID]

	// AccountBIC returns Block Issuer Credits of a specific account.
	AccountBIC(id iotago.AccountID) (account *iotago.AccountID, err error)

	// Mana is the stored and potential ana vale of an account collected on the UTXO layer - used by the Scheduler.
	Mana(id iotago.AccountID) (mana *Mana, err error)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
