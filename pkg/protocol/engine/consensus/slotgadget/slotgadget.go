package slotgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

type Gadget interface {
	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Interface
}
