package tests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/testsuite"
)

func TestLossOfAcceptanceFromGenesis(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThresholdLowerBound(10),
		testsuite.WithLivenessThresholdUpperBound(10),
		testsuite.WithMinCommittableAge(2),
		testsuite.WithMaxCommittableAge(4),
		testsuite.WithEpochNearingThreshold(2),
		testsuite.WithSlotsPerEpochExponent(3),
		testsuite.WithGenesisTimestampOffset(100*10),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")

	ts.Run(true, nil)

	block0 := ts.IssueValidationBlock("block0", node0,
		blockfactory.WithIssuingTime(ts.API.TimeProvider().SlotStartTime(50)),
		blockfactory.WithStrongParents(ts.BlockIDs("Genesis")...),
	)
	require.EqualValues(t, 48, block0.SlotCommitmentID().Slot())

	ts.AssertLatestCommitmentSlotIndex(48, ts.Nodes()...)
	ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes()...)
}
