package totalweightslotgadget

import "github.com/iotaledger/hive.go/runtime/options"

func WithSlotFinalizationThreshold(threshold float64) options.Option[Gadget] {
	return func(gadget *Gadget) {
		gadget.optsSlotFinalizationThreshold = threshold
	}
}
