package iotago

import (
	iotago "github.com/iotaledger/iota.go/v4"
)

type Input interface {
	ID() iotago.OutputID
}
