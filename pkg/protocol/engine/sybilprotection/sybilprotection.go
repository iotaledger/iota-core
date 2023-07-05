package sybilprotection

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SybilProtection is the minimal interface for the SybilProtection component of the IOTA protocol.
type SybilProtection interface {
	// RotateCommittee rotates the committee evaluating the given set of candidates to produce the new committee.
	RotateCommittee(epochIndex iotago.EpochIndex, candidates *account.Accounts) *account.SeatedAccounts

	// Committee returns the set of validators that is used to track confirmation.
	Committee(slotIndex iotago.SlotIndex) *account.SeatedAccounts

	// OnlineCommittee returns the set of online validators that is used to track acceptance.
	OnlineCommittee() *advancedset.AdvancedSet[account.SeatIndex]

	// SeatCount returns the number of seats in the SybilProtection.
	SeatCount() int

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
