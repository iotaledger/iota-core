package seatmanager

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SeatManager is the minimal interface for the SeatManager component of the IOTA protocol.
type SeatManager interface {
	// RotateCommittee rotates the committee evaluating the given set of candidates to produce the new committee.
	RotateCommittee(epochIndex iotago.EpochIndex, candidates *account.Accounts) *account.SeatedAccounts

	// SetCommittee sets the committee for a given slot.
	// This is used when re-using the same committee for consecutive epochs.
	SetCommittee(epochIndex iotago.EpochIndex, committee *account.Accounts)

	// ImportCommittee sets the committee for a given slot and marks the whole committee as active.
	// This is used when initializing committee after node startup (loaded from snapshot or database).
	ImportCommittee(epochIndex iotago.EpochIndex, committee *account.Accounts)

	// Committee returns the set of validators that is used to track confirmation.
	Committee(slotIndex iotago.SlotIndex) *account.SeatedAccounts

	// OnlineCommittee returns the set of online validators that is used to track acceptance.
	OnlineCommittee() ds.Set[account.SeatIndex]

	// SeatCount returns the number of seats in the SeatManager.
	SeatCount() int

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
