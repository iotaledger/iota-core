package models

import "github.com/iotaledger/iota.go/v4"

// Input contains details of an input.
type Input struct {
	OutputID iotago.OutputID
	Address  iotago.Address
}

// Output contains details of an output ID.
type Output struct {
	OutputID iotago.OutputID
	Address  iotago.Address
	Index    uint64
	Balance  iotago.BaseToken

	OutputStruct iotago.Output
}

// Outputs is a list of Output.
type Outputs []*Output
