package mempoolv1

import (
	"fmt"
	"runtime"
	memleakdebug "runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	mempooltests "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/tests"
	iotago "github.com/iotaledger/iota.go/v4"
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
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedPower](account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()).SelectAccounts())
	mempoolInstance := New[vote.MockedPower](mempooltests.VM, func(reference iotago.IndexedUTXOReferencer) *promise.Promise[mempool.State] {
		return ledgerState.ResolveState(reference.Ref())
	}, workers, conflictDAG)

	tf := mempooltests.NewTestFramework(t, mempoolInstance, conflictDAG, ledgerState, workers)

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
			tf.Instance.EvictUntil(iotago.SlotIndex(index))

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
	mempoolInstance.attachments.ForEach(func(index iotago.SlotIndex, storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *TransactionMetadata]) {
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
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedPower](account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()).SelectAccounts())

	return mempooltests.NewTestFramework(t, New[vote.MockedPower](mempooltests.VM, func(reference iotago.IndexedUTXOReferencer) *promise.Promise[mempool.State] {
		return ledgerState.ResolveState(reference.Ref())
	}, workers, conflictDAG), conflictDAG, ledgerState, workers)
}

func newForkingTestFramework(t *testing.T) *mempooltests.TestFramework {
	workers := workerpool.NewGroup(t.Name())

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.TransactionID{}, 0))
	conflictDAG := conflictdagv1.New[iotago.TransactionID, iotago.OutputID, vote.MockedPower](account.NewAccounts[iotago.AccountID, *iotago.AccountID](mapdb.NewMapDB()).SelectAccounts())

	return mempooltests.NewTestFramework(t, New[vote.MockedPower](mempooltests.VM, func(reference iotago.IndexedUTXOReferencer) *promise.Promise[mempool.State] {
		return ledgerState.ResolveState(reference.Ref())
	}, workers, conflictDAG, WithForkAllTransactions[vote.MockedPower](true)), conflictDAG, ledgerState, workers)
}
