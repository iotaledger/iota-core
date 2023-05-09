package mempoolv1

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestMemPool(t *testing.T) {
	mempooltests.TestAll(t, newTestFramework)
}

func newTestFramework(t *testing.T) *mempooltests.TestFramework {
	ledgerState := ledgertests.New(ledgertests.NewState(iotago.TransactionID{}, 0))

	return mempooltests.NewTestFramework(t, New[vote.MockedPower](mempooltests.VM, func(reference ledger.StateReference) *promise.Promise[ledger.State] {
		return ledgerState.ResolveState(reference.StateID())
	}, workerpool.NewGroup(t.Name())), func() {
		ledgerState.Cleanup()
	})
}
