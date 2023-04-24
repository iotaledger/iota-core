package mempoolv1

import (
	"fmt"
	"testing"
	"time"

	"iota-core/pkg/protocol/engine/ledger/mockedleger"
	"iota-core/pkg/protocol/engine/mempool"
	"iota-core/pkg/protocol/engine/mempool/mempooltests"
	"iota-core/pkg/protocol/engine/mempool/promise"
	"iota-core/pkg/protocol/engine/vm"
	vmtests "iota-core/pkg/protocol/engine/vm/tests"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
)

func NewTestFramework(t *testing.T) *mempooltests.TestFramework {
	ledgerState := mockedleger.New(vmtests.NewState(iotago.TransactionID{}, 0))

	return mempooltests.NewTestFramework(t, New(mempooltests.VM, func(reference mempool.StateReference) *promise.Promise[vm.State] {
		return ledgerState.ResolveState(reference.ReferencedStateID())
	}, workerpool.NewGroup(t.Name())))
}

func TestMemPool(t *testing.T) {
	tf := NewTestFramework(t)

	tf.Instance.Events().TransactionStored.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionStored", metadata.ID())
	})

	tf.Instance.Events().TransactionSolid.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionSolid", metadata.ID())
	})

	tf.Instance.Events().TransactionExecuted.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionExecuted", metadata.ID())
	})

	tf.Instance.Events().TransactionBooked.Hook(func(metadata mempool.TransactionMetadata) {
		fmt.Println("TransactionBooked", metadata.ID())
	})

	require.NoError(t, tf.ProcessTransaction("tx1", []string{"genesis"}, 1))

	time.Sleep(5 * time.Second)
}
