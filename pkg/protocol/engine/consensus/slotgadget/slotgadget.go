package slotgadget

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

type Gadget interface {
	Reset()

	module.Interface
}
