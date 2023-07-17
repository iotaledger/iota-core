package upgrade

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator interface {
	Commit(slot iotago.SlotIndex) (protocolParametersAndVersionsHash iotago.Identifier, err error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error

	RestoreFromDisk(slot iotago.SlotIndex) error

	module.Interface
}
