package mempoolv1

import (
	"testing"

	"iota-core/pkg/core/promise"
	"iota-core/pkg/protocol/engine/ledger"
	ledgertests "iota-core/pkg/protocol/engine/ledger/tests"
	"iota-core/pkg/protocol/engine/mempool/tests"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestMemPool(t *testing.T) {
	mempooltests.TestAll(t, newTestFramework)
}

func newTestFramework(t *testing.T) *mempooltests.TestFramework {
	ledgerState := ledgertests.New(ledgertests.NewState(iotago.TransactionID{}, 0))

	return mempooltests.NewTestFramework(t, New(mempooltests.VM, func(reference ledger.StateReference) *promise.Promise[ledger.State] {
		return ledgerState.ResolveState(reference.StateID())
	}, workerpool.NewGroup(t.Name())))
}
