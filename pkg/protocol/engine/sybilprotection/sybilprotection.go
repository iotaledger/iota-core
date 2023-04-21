package sybilprotection

import (
	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SybilProtection is the minimal interface for the SybilProtection component of the IOTA protocol.
type SybilProtection interface {
	// Accounts returns the weights of identities in the SybilProtection.
	Accounts() *account.Accounts[iotago.AccountID, *iotago.AccountID]

	// Committee returns the set of validators that is used to track confirmation.
	Committee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]

	// OnlineCommittee returns the set of online validators that is used to track acceptance.
	OnlineCommittee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]

	// LastCommittedSlot returns the last committed slot.
	LastCommittedSlot() iotago.SlotIndex

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
