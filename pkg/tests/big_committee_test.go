package tests

import (
	"fmt"
	"testing"

	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func TestBigCommittee(t *testing.T) {
	t.Skip("only for benchmarking performance")

	var (
		genesisSlot       iotago.SlotIndex = 0
		minCommittableAge iotago.SlotIndex = 2
		maxCommittableAge iotago.SlotIndex = 4
	)

	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				genesisSlot,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				3,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				minCommittableAge,
				maxCommittableAge,
				5,
			),
		),
	)
	defer ts.Shutdown()

	for i := 0; i < 32; i++ {
		nodeName := fmt.Sprintf("node%d", i)
		ts.AddValidatorNode(nodeName)
	}

	for i := 32; i < 50; i++ {
		nodeName := fmt.Sprintf("node%d", i)
		ts.AddNode(nodeName)
	}

	ts.Run(true)
	fmt.Println("TestBigCommittee starting to issue blocks...")

	ts.IssueBlocksAtSlots("", []iotago.SlotIndex{1, 2, 3, 4, 5}, 4, "Genesis", ts.Nodes(), true, false)

	ts.AssertNodeState(ts.Nodes(),
		testsuite.WithLatestCommitmentSlotIndex(3),
		testsuite.WithEqualStoredCommitmentAtIndex(3),
	)
}
