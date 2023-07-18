package upgrade

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator interface {
	TrackBlock(block *blocks.Block)
	Commit(slot iotago.SlotIndex) (protocolParametersAndVersionsHash iotago.Identifier, err error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error

	RestoreFromDisk(slot iotago.SlotIndex) error

	module.Interface
}
