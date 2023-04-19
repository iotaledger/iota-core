package slotgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	LastConfirmedSlot() iotago.SlotIndex

	module.Interface
}
