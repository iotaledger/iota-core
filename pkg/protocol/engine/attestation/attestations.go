package attestation

import (
	"io"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Attestations interface {
	// Get returns the attestations that are included in the commitment of the given slot as list.
	// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
	Get(index iotago.SlotIndex) (attestations []*iotago.Attestation, err error)

	// GetMap returns the attestations that are included in the commitment of the given slot as ads.Map.
	// If attestationCommitmentOffset=3 and commitment is 10, then the returned attestations are blocks from 7 to 10 that commit to at least 7.
	GetMap(index iotago.SlotIndex) (attestations ads.Map[iotago.AccountID, *iotago.Attestation], err error)
	AddAttestationFromValidationBlock(block *blocks.Block)
	Commit(index iotago.SlotIndex) (newCW uint64, attestationsRoot iotago.Identifier, err error)

	Import(reader io.ReadSeeker) (err error)
	Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error)
	Rollback(index iotago.SlotIndex) (err error)

	RestoreFromDisk() (err error)

	module.Interface
}
