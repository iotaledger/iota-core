package thresholdblockgadget

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

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

func WithConfirmationRatificationThreshold(confirmationRatificationThreshold iotago.SlotIndex) options.Option[Gadget] {
	return func(gadget *Gadget) {
		gadget.optsConfirmationRatificationThreshold = confirmationRatificationThreshold
	}
}
