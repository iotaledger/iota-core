package mempool

import (
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

// A generic interface over a state (like an output or a commitment).
type State interface {
	// The identifier of the state.
	StateID() StateID

	// The type of state.
	Type() utxoledger.StateType

	// Whether the state is read only.
	IsReadOnly() bool
}

// A thin wrapper around a resolved commitment.
type CommitmentInputState struct {
	Commitment *iotago.Commitment
}

func (s CommitmentInputState) StateID() StateID {
	return iotago.IdentifierFromData(lo.PanicOnErr(s.Commitment.MustID().Bytes()))
}

func (s CommitmentInputState) Type() utxoledger.StateType {
	return utxoledger.StateTypeCommitment
}

func (s CommitmentInputState) IsReadOnly() bool {
	return true
}

func CommitmentInputStateFromCommitment(commitment *iotago.Commitment) CommitmentInputState {
	return CommitmentInputState{
		Commitment: commitment,
	}
}
