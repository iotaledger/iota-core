package accounts

import (
	"crypto/ed25519"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TODO design apply diffs function for BIC and Holdings

// BlockIssuanceCredits is the minimal interface for the Accounts component of the IOTA protocol.
type BlockIssuanceCredits interface {
	// BIC returns Block Issuer Credits of all registered accounts.
	BIC() *account.Accounts[iotago.AccountID, *iotago.AccountID]

	// AccountBIC returns Block Issuer Credits of a specific account.
	AccountBIC(id iotago.AccountID) (account *Account, err error)

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

type Holdings interface {
	// Mana is the stored and potential value of an account collected on the UTXO layer - used by the Scheduler.
	Mana(id iotago.AccountID) (mana *Mana, err error)
}

type Account interface {
	ID() iotago.AccountID
	Credits() Credits
	IsPublicKeyAllowed(ed25519.PublicKey) bool
}
