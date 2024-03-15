package slotgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset(targetSlot iotago.SlotIndex)

	module.Module
}
