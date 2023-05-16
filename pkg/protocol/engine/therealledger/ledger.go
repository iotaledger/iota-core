package therealledger

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	Output(id iotago.OutputID) (*ledgerstate.Output, error)
	IsOutputSpent(outputID iotago.OutputID) (bool, error)
	CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, diff *ledgerstate.SlotDiff, err error)
	StateDiffs(index iotago.SlotIndex) (*ledgerstate.SlotDiff, error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error

	module.Interface
}
