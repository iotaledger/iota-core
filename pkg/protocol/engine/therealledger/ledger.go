package therealledger

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	Output(id iotago.OutputID) (*ledgerstate.Output, error)
	CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, err error)

	module.Interface
}
