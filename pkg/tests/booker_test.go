package tests

import (
	"testing"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/blockissuer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite"
)

func Test_IssuingTransactionsOutOfOrder(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	ts.Run(map[string][]options.Option[protocol.Protocol]{})

	node1.HookLogging()

	time.Sleep(time.Second)

	tx1 := ts.CreateTransaction("Tx1", 1, "Genesis")

	tx2 := ts.CreateTransaction("Tx2", 1, "Tx1:0")

	ts.RegisterBlock("block1", node1.IssueBlock("block1", blockissuer.WithPayload(tx2)))
	ts.Wait(node1)

	ts.AssertTransactionsExist([]string{"Tx2"}, true, node1)
	ts.AssertTransactionsExist([]string{"Tx1"}, false, node1)

	ts.AssertTransactionsInCacheBooked([]string{"Tx2"}, false, node1)

	ts.RegisterBlock("block2", node1.IssueBlock("block2", blockissuer.WithPayload(tx1)))
	ts.Wait(node1)

	ts.AssertTransactionsExist([]string{"Tx1", "Tx2"}, true, node1)
	ts.AssertTransactionsInCacheBooked([]string{"Tx1", "Tx2"}, true, node1)
	ts.AssertBlocksInCacheConflicts(map[string][]string{
		"block1": {"Tx2"},
		"block2": {"Tx1"},
	}, node1)

	ts.AssertTransactionInCacheConflicts(map[string][]string{
		"Tx2": {"Tx2"},
		"Tx1": {"Tx1"},
	}, node1)
}

func Test_DoubleSpend(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1)
	node2 := ts.AddValidatorNode("node2", 1)

	ts.Run(map[string][]options.Option[protocol.Protocol]{})

	node1.HookLogging()

	time.Sleep(time.Second)
	//Create and issue double spends
	{
		tx1 := ts.CreateTransaction("Tx1", 1, "Genesis")
		tx2 := ts.CreateTransaction("Tx2", 1, "Genesis")

		ts.RegisterBlock("block1", node1.IssueBlock("block1", blockissuer.WithPayload(tx1)))
		ts.RegisterBlock("block2", node1.IssueBlock("block2", blockissuer.WithPayload(tx2)))
		ts.Wait(node1, node2)

		ts.AssertTransactionsExist([]string{"Tx1", "Tx2"}, true, node1, node2)
		ts.AssertTransactionsInCacheBooked([]string{"Tx1", "Tx2"}, true, node1, node2)
		ts.AssertTransactionsInCachePending([]string{"Tx1", "Tx2"}, true, node1, node2)
		ts.AssertBlocksInCacheConflicts(map[string][]string{
			"block1": {"Tx1"},
			"block2": {"Tx2"},
		}, node1, node2)

		ts.AssertTransactionInCacheConflicts(map[string][]string{
			"Tx2": {"Tx2"},
			"Tx1": {"Tx1"},
		}, node1, node2)
	}

	// Issue some more blocks and assert that conflicts are propagated to blocks.
	{
		references := make(model.ParentReferences)
		references[model.StrongParentType] = ts.BlockIDs("block1")
		ts.RegisterBlock("block3", node1.IssueBlock("block3", blockissuer.WithReferences(references)))

		references = make(model.ParentReferences)
		references[model.StrongParentType] = ts.BlockIDs("block2")
		ts.RegisterBlock("block4", node1.IssueBlock("block4", blockissuer.WithReferences(references)))
		ts.Wait(node1, node2)

		ts.AssertBlocksInCacheConflicts(map[string][]string{
			"block3": {"Tx1"},
			"block4": {"Tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending([]string{"Tx1", "Tx2"}, true, node1, node2)
	}

	// Issue an invalid block and assert that its vote is not cast.
	{
		references := make(model.ParentReferences)
		references[model.StrongParentType] = ts.BlockIDs("block3", "block4")
		ts.RegisterBlock("block5", node2.IssueBlock("block5", blockissuer.WithReferences(references)))
		ts.Wait(node1, node2)

		ts.AssertTransactionsInCachePending([]string{"Tx1", "Tx2"}, true, node1, node2)
	}

	// Issue a valid block that resolves the conflict.
	{
		references := make(model.ParentReferences)
		references[model.StrongParentType] = ts.BlockIDs("block3", "block4")
		references[model.ShallowLikeParentType] = ts.BlockIDs("block2")

		ts.RegisterBlock("block6", node2.IssueBlock("block6", blockissuer.WithReferences(references)))
		ts.Wait(node1, node2)

		ts.AssertBlocksInCacheConflicts(map[string][]string{
			"block6": {"Tx2"},
		}, node1, node2)
		ts.AssertTransactionsInCachePending([]string{"Tx1", "Tx2"}, false, node1, node2)
		ts.AssertTransactionsInCacheAccepted([]string{"Tx2"}, true, node1, node2)
		ts.AssertTransactionsInCacheRejected([]string{"Tx1"}, true, node1, node2)

	}
}
