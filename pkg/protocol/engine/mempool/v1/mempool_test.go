package mempoolv1

import (
	"fmt"
	"runtime"
	memleakdebug "runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestMemPoolV1_InterfaceWithoutForkingEverything(t *testing.T) {
	mempooltests.TestAllWithoutForkingEverything(t, newTestFramework)
}

func TestMemPoolV1_InterfaceWithForkingEverything(t *testing.T) {
	mempooltests.TestAllWithForkingEverything(t, newForkingTestFramework)
}

func TestMempoolV1_ResourceCleanup(t *testing.T) {
	workers := workerpool.NewGroup(t.Name())

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.TransactionID{}, 0))
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedRank](func() int { return 0 })
	mempoolInstance := New[vote.MockedRank](mempooltests.VM, func(reference iotago.Input) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference.(*iotago.UTXOInput).OutputID())
	}, workers, conflictDAG, api.SingleVersionProvider(tpkg.TestAPI), func(error) {})

	tf := mempooltests.NewTestFramework(t, mempoolInstance, conflictDAG, ledgerState, workers)

	issueTransactions := func(startIndex, transactionCount int, prevStateAlias string) (int, string) {
		index := startIndex
		for ; index < startIndex+transactionCount; index++ {
			signedTxAlias := fmt.Sprintf("tx%d-signed", index)
			txAlias := fmt.Sprintf("tx%d", index)
			blockAlias := fmt.Sprintf("block%d", index)
			tf.CreateTransaction(txAlias, []string{prevStateAlias}, 2)

			require.NoError(t, tf.AttachTransaction(signedTxAlias, txAlias, blockAlias, iotago.SlotIndex(index)))
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

	txIndex, prevStateAlias := issueTransactions(1, 10, "genesis")
	tf.WaitChildren()

	require.Equal(t, 0, mempoolInstance.cachedTransactions.Size())
	require.Equal(t, 0, mempoolInstance.stateDiffs.Size())
	require.Equal(t, 0, mempoolInstance.cachedStateRequests.Size())

	txIndex, prevStateAlias = issueTransactions(txIndex, 10, prevStateAlias)
	tf.WaitChildren()

	require.Equal(t, 0, mempoolInstance.cachedTransactions.Size())
	require.Equal(t, 0, mempoolInstance.stateDiffs.Size())
	require.Equal(t, 0, mempoolInstance.cachedStateRequests.Size())

	attachmentsSlotCount := 0
	mempoolInstance.attachments.ForEach(func(index iotago.SlotIndex, storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *SignedTransactionMetadata]) {
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
	workers := workerpool.NewGroup(t.Name())

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.TransactionID{}, 0))
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedRank](account.NewAccounts().SelectCommittee().SeatCount)

	return mempooltests.NewTestFramework(t, New[vote.MockedRank](mempooltests.VM, func(reference iotago.Input) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference.(*iotago.UTXOInput).OutputID())
	}, workers, conflictDAG, api.SingleVersionProvider(tpkg.TestAPI), func(error) {}), conflictDAG, ledgerState, workers)
}

func newForkingTestFramework(t *testing.T) *mempooltests.TestFramework {
	workers := workerpool.NewGroup(t.Name())

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.TransactionID{}, 0))
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedRank](account.NewAccounts().SelectCommittee().SeatCount)

	return mempooltests.NewTestFramework(t, New[vote.MockedRank](mempooltests.VM, func(reference iotago.Input) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference.(*iotago.UTXOInput).OutputID())
	}, workers, conflictDAG, api.SingleVersionProvider(tpkg.TestAPI), func(error) {}, WithForkAllTransactions[vote.MockedRank](true)), conflictDAG, ledgerState, workers)
}
