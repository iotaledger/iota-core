package iotago

import iotago "github.com/iotaledger/iota.go/v4"

type Output interface {
	ID() iotago.OutputID
}
