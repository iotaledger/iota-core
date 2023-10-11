package models

import (
	"crypto/ed25519"

	"github.com/iotaledger/iota-core/pkg/blockhandler"
	iotago "github.com/iotaledger/iota.go/v4"
)

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

type AccountStatus uint8

const (
	AccountPending AccountStatus = iota
	AccountReady
)

type AccountData struct {
	Alias    string
	Status   AccountStatus
	Account  blockhandler.Account
	OutputID iotago.OutputID
	Index    uint64
}

type AccountState struct {
	Alias      string             `serix:"0,lengthPrefixType=uint8"`
	AccountID  iotago.AccountID   `serix:"2"`
	PrivateKey ed25519.PrivateKey `serix:"3,lengthPrefixType=uint8"`
	OutputID   iotago.OutputID    `serix:"4"`
	Index      uint64             `serix:"5"`
}
