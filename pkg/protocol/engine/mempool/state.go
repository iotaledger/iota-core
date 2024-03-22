package mempool

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// A generic interface over a state (like an output or a commitment).
type State interface {
	// The identifier of the state.
	StateID() StateID

	// The type of state.
	Type() StateType

	// Whether the state is read only.
	IsReadOnly() bool

	// SlotBooked returns the slot index of the state if it is booked, otherwise -1.
	SlotBooked() iotago.SlotIndex
}

// A thin wrapper around a resolved commitment.
type CommitmentInputState struct {
	Commitment *iotago.Commitment
}

func (s CommitmentInputState) StateID() StateID {
	return iotago.IdentifierFromData(lo.PanicOnErr(s.Commitment.MustID().Bytes()))
}

func (s CommitmentInputState) Type() StateType {
	return StateTypeCommitment
}

func (s CommitmentInputState) IsReadOnly() bool {
	return true
}

func (s CommitmentInputState) SlotBooked() iotago.SlotIndex {
	return s.Commitment.Slot
}

func CommitmentInputStateFromCommitment(commitment *iotago.Commitment) CommitmentInputState {
	return CommitmentInputState{
		Commitment: commitment,
	}
}
