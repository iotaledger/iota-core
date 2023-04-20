package thresholdblockgadget

import "github.com/iotaledger/hive.go/runtime/options"

func WithAcceptanceThreshold(acceptanceThreshold float64) options.Option[Gadget] {
	return func(gadget *Gadget) {
		gadget.optsAcceptanceThreshold = acceptanceThreshold
	}
}

func WithConfirmationThreshold(confirmationThreshold float64) options.Option[Gadget] {
	return func(gadget *Gadget) {
		gadget.optsConfirmationThreshold = confirmationThreshold
	}
}
