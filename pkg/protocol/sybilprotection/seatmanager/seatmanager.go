package seatmanager

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SeatManager is the minimal interface for the SeatManager component of the IOTA protocol.
type SeatManager interface {
	// RotateCommittee rotates the committee evaluating the given set of candidates to produce the new committee.
	RotateCommittee(epoch iotago.EpochIndex, candidates accounts.AccountsData) (*account.SeatedAccounts, error)

	// ReuseCommittee reuses the committee from a previous epoch.
	ReuseCommittee(prevEpoch iotago.EpochIndex, targetEpoch iotago.EpochIndex) (*account.SeatedAccounts, error)

	// InitializeCommittee initializes the committee for the current slot by marking whole or a subset of the committee as active.
	// This is used when initializing committee after node startup (loaded from snapshot or database).
	InitializeCommittee(epoch iotago.EpochIndex, activityTime time.Time) error

	// CommitteeInSlot returns the set of validators that is used to track confirmation at a given slot.
	CommitteeInSlot(slot iotago.SlotIndex) (*account.SeatedAccounts, bool)

	// CommitteeInEpoch returns the set of validators that is used to track confirmation in a given epoch.
	CommitteeInEpoch(epoch iotago.EpochIndex) (*account.SeatedAccounts, bool)

	// OnlineCommittee returns the set of online validators that is used to track acceptance.
	OnlineCommittee() ds.Set[account.SeatIndex]

	// SeatCountInSlot returns the number of seats in the SeatManager for the given slot's epoch.
	SeatCountInSlot(slot iotago.SlotIndex) int

	// SeatCountInEpoch returns the number of seats in the SeatManager for the given epoch.
	SeatCountInEpoch(epoch iotago.EpochIndex) int

	// Interface embeds the required methods of the module.Module.
	module.Module
}
