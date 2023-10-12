package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/snapshotcreator"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TopStakersRotation(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithLivenessThresholdLowerBound(10),
		testsuite.WithLivenessThresholdUpperBound(10),
		testsuite.WithMinCommittableAge(3),
		testsuite.WithMaxCommittableAge(4),
		testsuite.WithEpochNearingThreshold(5),
		testsuite.WithSlotsPerEpochExponent(4),
		testsuite.WithGenesisTimestampOffset(100*10),
		testsuite.WithSnapshotOptions(
			snapshotcreator.WithSeatManagerProvider(
				topstakers.NewProvider(
					topstakers.WithSeatCount(3),
				),
			),
		),
	)
	defer ts.Shutdown()

	ts.AddValidatorNode("node1", 1_000_002)
	ts.AddValidatorNode("node2", 1_000_001)
	ts.AddValidatorNode("node3", 1_000_000)
	ts.AddValidatorNode("node4")
	ts.AddValidatorNode("node5")
	ts.AddValidatorNode("node6")

	nodeOptions := make(map[string][]options.Option[protocol.Protocol])

	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(
						topstakers.WithSeatCount(3),
					),
				),
			),
		)}
	}
	ts.Run(true, nodeOptions)

	for _, node := range ts.Nodes() {
		nodeOptions[node.Name] = []options.Option[protocol.Protocol]{protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(topstakers.WithSeatCount(3)),
				),
			),
		)}
	}

	// TODO: replace with CandidacyPayload
	//pointOfNoReturn := ts.API.TimeProvider().EpochEnd(0) - ts.API.ProtocolParameters().MaxCommittableAge()
	ts.IssueBlocksAtSlots("candidate:", []iotago.SlotIndex{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17}, 4, "Genesis", ts.Nodes(), true, nil)
	//ts.IssueBlocksAtSlots("commit:", []iotago.SlotIndex{pointOfNoReturn - 1, pointOfNoReturn, pointOfNoReturn + 1}, 4, "candidate:9", ts.Nodes("node1", "node2", "node3"), true, nil)

	ts.AssertLatestFinalizedSlot(13, ts.Nodes()...)
}
