package attestation

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Attestation interface {
	Get(index iotago.SlotIndex) (attestations *ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], err error)
	AddAttestationFromBlock(block *blocks.Block)
	Commit(index iotago.SlotIndex) (newCW uint64, attestationsMap *ads.Map[iotago.AccountID, iotago.Attestation, *iotago.AccountID, *iotago.Attestation], err error)

	// TODO: integrate into snapshot
	//  Import(reader io.ReadSeeker) (err error)
	//  Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error)

	module.Interface
}
