package mempool

import (
	"iota-core/pkg/protocol/engine/ledger"
)

type VM func(transaction Transaction, inputs []ledger.Output, gasLimit ...uint64) (outputs []ledger.Output, err error)
