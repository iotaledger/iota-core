package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection/poa"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestProtocol_EngineSwitching(t *testing.T) {
	ts := testsuite.NewTestSuite(t)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNodeToPartition("node1", 75, "P1")
	node2 := ts.AddValidatorNodeToPartition("node2", 75, "P1")
	node3 := ts.AddValidatorNodeToPartition("node3", 25, "P2")
	node4 := ts.AddValidatorNodeToPartition("node4", 25, "P2")

	ts.Run(map[string][]options.Option[protocol.Protocol]{
		"node1": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node1.AccountID, node2.AccountID))),
		},
		"node2": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node1.AccountID, node2.AccountID))),
		},
		"node3": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node3.AccountID, node4.AccountID))),
		},
		"node4": {
			protocol.WithSybilProtectionProvider(poa.NewProvider(ts.Validators(), poa.WithOnlineCommitteeStartup(node3.AccountID, node4.AccountID))),
		},
	})
	ts.HookLogging()

	ts.Wait()

	// Verify all nodes have the expected states.
	ts.AssertSnapshotImported(true, ts.Nodes()...)
	ts.AssertProtocolParameters(ts.ProtocolParameters, ts.Nodes()...)
	ts.AssertLatestCommitment(iotago.NewEmptyCommitment(), ts.Nodes()...)
	ts.AssertLatestStateMutationSlot(0, ts.Nodes()...)
	ts.AssertLatestFinalizedSlot(0, ts.Nodes()...)
	ts.AssertChainID(iotago.NewEmptyCommitment().MustID(), ts.Nodes()...)

	ts.AssertStorageCommitments([]*iotago.Commitment{iotago.NewEmptyCommitment()}, ts.Nodes()...)

	ts.AssertSybilProtectionCommittee(map[iotago.AccountID]int64{
		node1.AccountID: 75,
		node2.AccountID: 75,
		node3.AccountID: 25,
		node4.AccountID: 25,
	}, ts.Nodes()...)
	ts.AssertSybilProtectionOnlineCommittee(map[iotago.AccountID]int64{
		node1.AccountID: 75,
		node2.AccountID: 75,
	}, node1, node2)
	ts.AssertSybilProtectionOnlineCommittee(map[iotago.AccountID]int64{
		node3.AccountID: 25,
		node4.AccountID: 25,
	}, node3, node4)

	// Issue blocks on partition 1.
	{
		ts.IssueBlockAtSlot("P1.A", 5, node1, iotago.EmptyBlockID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.B", 6, node2, ts.Block("P1.A").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.C", 7, node1, ts.Block("P1.B").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.D", 8, node2, ts.Block("P1.C").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.E", 9, node1, ts.Block("P1.D").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.F", 10, node2, ts.Block("P1.E").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P1.G", 11, node1, ts.Block("P1.F").ID())

		ts.WaitWithDelay(1 * time.Second) // Give some time for the blocks to arrive over the network

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), true, node1, node2)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P1"), false, node3, node4)

		ts.AssertBlocksAccepted(ts.Blocks("P1.A", "P1.B", "P1.C", "P1.D", "P1.E", "P1.F"), true, node1, node2)
		ts.AssertBlocksAccepted(ts.Blocks("P1.G"), false, node1, node2) // block not referenced yet
	}

	// Issue blocks on partition 2.
	{
		ts.IssueBlockAtSlot("P2.A", 5, node3, iotago.EmptyBlockID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.B", 6, node4, ts.Block("P2.A").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.C", 7, node3, ts.Block("P2.B").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.D", 8, node4, ts.Block("P2.C").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.E", 9, node3, ts.Block("P2.D").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.F", 10, node4, ts.Block("P2.E").ID())
		ts.Wait()
		ts.IssueBlockAtSlot("P2.G", 11, node3, ts.Block("P2.F").ID())

		ts.WaitWithDelay(1 * time.Second) // Give some time for the blocks to arrive over the network

		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), true, node3, node4)
		ts.AssertBlocksExist(ts.BlocksWithPrefix("P2"), false, node1, node2)

		ts.AssertBlocksAccepted(ts.Blocks("P2.A", "P2.B", "P2.C", "P2.D", "P2.E", "P2.F"), true, node3, node4)
		ts.AssertBlocksAccepted(ts.Blocks("P2.G"), false, node3, node4) // block not referenced yet
	}

	// Both partitions should have committed slot 8 and have different commitments
	{
		ts.Wait()
		ts.AssertLatestCommitmentSlotIndex(8, ts.Nodes()...)

		require.Equal(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node2.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		require.Equal(t, node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node4.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		require.NotEqual(t, node1.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment(), node3.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
	}

	// Merge the partitions
	{
		ts.Network.MergePartitionsToMain()
		fmt.Println("\n=========================\nMerged network partitions\n=========================")
	}

	// TODO: engine switching is currently not yet implemented
	// wg := &sync.WaitGroup{}
	//
	// // Issue blocks after merging the networks
	// {
	// 	wg.Add(4)
	//
	// 	node1.IssueActivity(25*time.Second, wg)
	// 	node2.IssueActivity(25*time.Second, wg)
	// 	node3.IssueActivity(25*time.Second, wg)
	// 	node4.IssueActivity(25*time.Second, wg)
	// }
	//
	// wg.Wait()
}
