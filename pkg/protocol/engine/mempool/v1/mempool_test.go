package mempoolv1

import (
	"fmt"
	"runtime"
	memleakdebug "runtime/debug"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestMemPoolV1_Interface(t *testing.T) {
	mempooltests.TestAll(t, newTestFramework)
}

func TestMempoolV1_ResourceCleanup(t *testing.T) {
	ledgerState := ledgertests.New(ledgertests.NewState(iotago.TransactionID{}, 0))

	mempoolInstance := New[vote.MockedPower](mempooltests.VM, func(reference ledger.StateReference) *promise.Promise[ledger.State] {
		return ledgerState.ResolveState(reference.StateID())
	}, workerpool.NewGroup(t.Name()))

	tf := mempooltests.NewTestFramework(t, mempoolInstance, ledgerState)

	issueTransactions := func(startIndex, transactionCount int, prevStateAlias string) (int, string) {
		index := startIndex
		for ; index < startIndex+transactionCount; index++ {
			txAlias := fmt.Sprintf("tx%d", index)
			blockAlias := fmt.Sprintf("block%d", index)
			tf.CreateTransaction(txAlias, []string{prevStateAlias}, 2)

			require.NoError(t, tf.AttachTransaction(txAlias, blockAlias, iotago.SlotIndex(index)))
			tf.RequireBooked(txAlias)

			tf.MarkAttachmentIncluded(blockAlias)

			prevStateAlias = fmt.Sprintf("tx%d:0", index)

			tf.CommitSlot(iotago.SlotIndex(index))
			tf.Instance.Evict(iotago.SlotIndex(index))

			require.Nil(t, mempoolInstance.attachments.Get(iotago.SlotIndex(index), false))

		}
		return index, prevStateAlias
	}

	fmt.Println("Memory report before:")
	fmt.Println(memanalyzer.MemoryReport(tf))

	txIndex, prevStateAlias := issueTransactions(1, 10000, "genesis")

	require.Equal(t, 0, mempoolInstance.cachedTransactions.Size())
	require.Equal(t, 0, mempoolInstance.stateDiffs.Size())
	require.Equal(t, 0, mempoolInstance.cachedStateRequests.Size())

	time.Sleep(1 * time.Second)

	txIndex, prevStateAlias = issueTransactions(txIndex, 10000, prevStateAlias)

	require.Equal(t, 0, mempoolInstance.cachedTransactions.Size())
	require.Equal(t, 0, mempoolInstance.stateDiffs.Size())
	require.Equal(t, 0, mempoolInstance.cachedStateRequests.Size())

	attachmentsSlotCount := 0
	mempoolInstance.attachments.ForEach(func(index iotago.SlotIndex, storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *TransactionWithMetadata]) {
		attachmentsSlotCount++
	})

	require.Equal(t, 0, attachmentsSlotCount)

	tf.Cleanup()

	runtime.GC()
	memleakdebug.FreeOSMemory()

	fmt.Println("------------\nMemory report after:")
	fmt.Println(memanalyzer.MemoryReport(tf))
}

func newTestFramework(t *testing.T) *mempooltests.TestFramework {
	ledgerState := ledgertests.New(ledgertests.NewState(iotago.TransactionID{}, 0))

	return mempooltests.NewTestFramework(t, New[vote.MockedPower](mempooltests.VM, func(reference ledger.StateReference) *promise.Promise[ledger.State] {
		return ledgerState.ResolveState(reference.StateID())
	}, workerpool.NewGroup(t.Name())), ledgerState)
}
