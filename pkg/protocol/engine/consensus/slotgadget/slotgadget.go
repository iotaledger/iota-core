package slotgadget

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget interface {
	LatestFinalizedSlot() iotago.SlotIndex

	LatestFinalizedSlotR() reactive.Variable[iotago.SlotIndex]

	module.Interface
}
