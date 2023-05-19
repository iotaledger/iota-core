package mock

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/utxoledger"
	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
)

func NewMockedLedgerProvider() module.Provider[*engine.Engine, ledger.Ledger] {
	return module.Provide(func(e *engine.Engine) ledger.Ledger {
		l := utxoledger.New(e.Workers.CreateGroup("MockedLedger"), e.Storage.Ledger(), mempooltests.VM, e.API, e.SybilProtection.OnlineCommittee(), e.ErrorHandler("ledger"))

		// TODO: should this attach to RatifiedAccepted instead?
		e.Events.BlockGadget.BlockAccepted.Hook(l.BlockAccepted)

		return l
	})
}
