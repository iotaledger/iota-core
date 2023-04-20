package notarization

import (
	"io"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Notarization interface {
	Attestations() Attestations

	// IsBootstrapped returns if notarization finished committing all pending slots up to the current acceptance time.
	IsBootstrapped() bool

	Import(reader io.ReadSeeker) (err error)

	Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error)

	module.Interface
}

type Attestations interface {
	Get(index iotago.SlotIndex) (attestations *ads.Map[identity.ID, iotago.Attestation, *identity.ID, *iotago.Attestation], err error)

	// LastCommittedSlot returns the last committed slot.
	LastCommittedSlot() (index iotago.SlotIndex)

	module.Interface
}
