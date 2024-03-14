package upgrade

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator interface {
	TrackValidationBlock(block *blocks.Block)
	Commit(slot iotago.SlotIndex) (protocolParametersAndVersionsHash iotago.Identifier, err error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error

	RestoreFromDisk(slot iotago.SlotIndex) error

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
