package mempoolv1

import (
	"fmt"
	"runtime"
	memleakdebug "runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/memanalyzer"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag/spenddagv1"
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

	mutationsFunc := func(index iotago.SlotIndex) (kvstore.KVStore, error) {
		return mapdb.NewMapDB(), nil
	}

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.EmptyTransactionID, 0))
	spenddag := spenddagv1.New[iotago.TransactionID, mempool.StateID, vote.MockedRank](func() int { return 0 })
	memPoolInstance := New[vote.MockedRank](new(mempooltests.VM), func(reference mempool.StateReference) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference)
	}, mutationsFunc, workers, spenddag, api.SingleVersionProvider(tpkg.TestAPI), func(error) {})

	tf := mempooltests.NewTestFramework(t, memPoolInstance, spenddag, ledgerState, workers)

	issueTransactions := func(startIndex, transactionCount int, prevStateAlias string) (int, string) {
		index := startIndex
		for ; index < startIndex+transactionCount; index++ {
			signedTxAlias := fmt.Sprintf("tx%d-signed", index)
			txAlias := fmt.Sprintf("tx%d", index)
			blockAlias := fmt.Sprintf("block%d", index)
			tf.CreateSignedTransaction(txAlias, []string{prevStateAlias}, 2)

			require.NoError(t, tf.AttachTransaction(signedTxAlias, txAlias, blockAlias, iotago.SlotIndex(index)))
			tf.RequireBooked(txAlias)

			tf.MarkAttachmentIncluded(blockAlias)

			prevStateAlias = fmt.Sprintf("tx%d:0", index)

			tf.CommitSlot(iotago.SlotIndex(index))
			tf.Instance.Evict(iotago.SlotIndex(index))

			require.Nil(t, memPoolInstance.attachments.Get(iotago.SlotIndex(index), false))

		}
		return index, prevStateAlias
	}

	fmt.Println("Memory report before:")
	fmt.Println(memanalyzer.MemoryReport(tf))

	txIndex, prevStateAlias := issueTransactions(1, 10, "genesis")
	tf.WaitChildren()

	require.Equal(t, 10, memPoolInstance.cachedTransactions.Size())
	require.Equal(t, 10, memPoolInstance.delayedTransactionEviction.Size())
	require.Equal(t, 0, memPoolInstance.stateDiffs.Size())
	require.Equal(t, 10, memPoolInstance.delayedOutputStateEviction.Size())
	// genesis output is never committed or orphaned, so it is not evicted as it has still a non-evicted spender.
	require.Equal(t, 11, memPoolInstance.cachedStateRequests.Size())

	txIndex, prevStateAlias = issueTransactions(txIndex, 10, prevStateAlias)
	tf.WaitChildren()

	require.Equal(t, 20, memPoolInstance.cachedTransactions.Size())
	require.Equal(t, 20, memPoolInstance.delayedTransactionEviction.Size())
	require.Equal(t, 0, memPoolInstance.stateDiffs.Size())
	require.Equal(t, 20, memPoolInstance.delayedOutputStateEviction.Size())
	require.Equal(t, 21, memPoolInstance.cachedStateRequests.Size())

	txIndex, prevStateAlias = issueTransactions(txIndex, 10, prevStateAlias)
	tf.WaitChildren()

	// Cached transactions stop increasing after 20, because eviction is delayed by MCA.
	require.Equal(t, 20, memPoolInstance.cachedTransactions.Size())
	require.Equal(t, 20, memPoolInstance.delayedTransactionEviction.Size())
	require.Equal(t, 0, memPoolInstance.stateDiffs.Size())
	require.Equal(t, 20, memPoolInstance.delayedOutputStateEviction.Size())
	require.Equal(t, 21, memPoolInstance.cachedStateRequests.Size())

	txIndex, _ = issueTransactions(txIndex, 10, prevStateAlias)
	tf.WaitChildren()

	require.Equal(t, 20, memPoolInstance.cachedTransactions.Size())
	require.Equal(t, 20, memPoolInstance.delayedTransactionEviction.Size())
	require.Equal(t, 0, memPoolInstance.stateDiffs.Size())
	require.Equal(t, 20, memPoolInstance.delayedOutputStateEviction.Size())
	require.Equal(t, 21, memPoolInstance.cachedStateRequests.Size())

	for index := txIndex; index <= txIndex+20; index++ {
		tf.CommitSlot(iotago.SlotIndex(index))
		tf.Instance.Evict(iotago.SlotIndex(index))
	}

	// Genesis output is also finally evicted thanks to the fact that all its spenders got evicted.
	require.Equal(t, 0, memPoolInstance.cachedTransactions.Size())
	require.Equal(t, 0, memPoolInstance.delayedTransactionEviction.Size())
	require.Equal(t, 0, memPoolInstance.stateDiffs.Size())
	require.Equal(t, 0, memPoolInstance.delayedOutputStateEviction.Size())
	require.Equal(t, 0, memPoolInstance.cachedStateRequests.Size())

	attachmentsSlotCount := 0
	memPoolInstance.attachments.ForEach(func(index iotago.SlotIndex, storage *shrinkingmap.ShrinkingMap[iotago.BlockID, *SignedTransactionMetadata]) {
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

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.EmptyTransactionID, 0))
	spenddag := spenddagv1.New[iotago.TransactionID, mempool.StateID, vote.MockedRank](account.NewAccounts().SelectCommittee().SeatCount)

	mutationsFunc := func(index iotago.SlotIndex) (kvstore.KVStore, error) {
		return mapdb.NewMapDB(), nil
	}

	return mempooltests.NewTestFramework(t, New[vote.MockedRank](new(mempooltests.VM), func(reference mempool.StateReference) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference)
	}, mutationsFunc, workers, spenddag, api.SingleVersionProvider(tpkg.TestAPI), func(error) {}), spenddag, ledgerState, workers)
}

func newForkingTestFramework(t *testing.T) *mempooltests.TestFramework {
	workers := workerpool.NewGroup(t.Name())

	ledgerState := ledgertests.New(ledgertests.NewMockedState(iotago.EmptyTransactionID, 0))
	spenddag := spenddagv1.New[iotago.TransactionID, mempool.StateID, vote.MockedRank](account.NewAccounts().SelectCommittee().SeatCount)

	mutationsFunc := func(index iotago.SlotIndex) (kvstore.KVStore, error) {
		return mapdb.NewMapDB(), nil
	}

	return mempooltests.NewTestFramework(t, New[vote.MockedRank](new(mempooltests.VM), func(reference mempool.StateReference) *promise.Promise[mempool.State] {
		return ledgerState.ResolveOutputState(reference)
	}, mutationsFunc, workers, spenddag, api.SingleVersionProvider(tpkg.TestAPI), func(error) {}), spenddag, ledgerState, workers)
}
