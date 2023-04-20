package slotgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	LatestFinalizedSlot() iotago.SlotIndex

	module.Interface
}
