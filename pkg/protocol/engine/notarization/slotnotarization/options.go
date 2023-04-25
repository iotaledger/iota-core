package slotnotarization

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

// WithMinCommittableSlotAge specifies how old a slot has to be for it to be committable.
func WithMinCommittableSlotAge(age iotago.SlotIndex) options.Option[Manager] {
	return func(manager *Manager) {
		manager.optsMinCommittableSlotAge = age
	}
}
