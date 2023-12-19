package mempool

import (
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// A reference to a state (like an output or a commitment).
type StateReference interface {
	// The identifier of the state to which it resolves.
	ReferencedStateID() iotago.Identifier

	// The type of state.
	Type() StateType
}

// A thin wrapper around a UTXO input.
type UTXOInputStateRef struct {
	Input *iotago.UTXOInput
}

func (r UTXOInputStateRef) ReferencedStateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(r.Input.OutputID().Bytes()))
}

func (r UTXOInputStateRef) Type() StateType {
	return StateTypeUTXOInput
}

func UTXOInputStateRefFromInput(input *iotago.UTXOInput) UTXOInputStateRef {
	return UTXOInputStateRef{
		Input: input,
	}
}

// A thin wrapper around a Commitment input.
type CommitmentInputStateRef struct {
	Input *iotago.CommitmentInput
}

func (r CommitmentInputStateRef) ReferencedStateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(r.Input.CommitmentID.Bytes()))
}

func (r CommitmentInputStateRef) Type() StateType {
	return StateTypeCommitment
}

func CommitmentInputStateRefFromInput(input *iotago.CommitmentInput) CommitmentInputStateRef {
	return CommitmentInputStateRef{
		Input: input,
	}
}
