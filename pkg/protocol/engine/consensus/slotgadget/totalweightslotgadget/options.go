package totalweightslotgadget

import "github.com/iotaledger/hive.go/runtime/options"

func WithSlotConfirmationThreshold(acceptanceThreshold float64) options.Option[Gadget] {
	return func(gadget *Gadget) {
		gadget.optsSlotConfirmationThreshold = acceptanceThreshold
	}
}
