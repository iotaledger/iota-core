package therealledger

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	Output(id iotago.OutputID) (*ledgerstate.Output, error)
	IsOutputUnspent(outputID iotago.OutputID) (bool, error)
	Spent(outputID iotago.OutputID) (*ledgerstate.Spent, error)
	CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, err error)
	StateDiffs(index iotago.SlotIndex) (*ledgerstate.SlotDiff, error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error

	module.Interface
}
