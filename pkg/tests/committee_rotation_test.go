package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/snapshotcreator"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TopStakersRotation(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				4,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				3,
				4,
				5,
			),
		),
		testsuite.WithSnapshotOptions(
			snapshotcreator.WithSeatManagerProvider(
				topstakers.NewProvider(
					topstakers.WithSeatCount(3),
				),
			),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", 1_000_006)
	ts.AddValidatorNode("node2", 1_000_005)
	ts.AddValidatorNode("node3", 1_000_004)
	ts.AddValidatorNode("node4", 1_000_003)
	ts.AddValidatorNode("node5", 1_000_002)
	ts.AddValidatorNode("node6", 1_000_001)
	ts.AddGenesisWallet("default", node1)

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
	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		ts.Node("node1").Validator.AccountID,
		ts.Node("node2").Validator.AccountID,
		ts.Node("node3").Validator.AccountID,
	}, ts.Nodes()...)

	ts.IssueBlocksAtSlots("wave-1:", []iotago.SlotIndex{1, 2, 3, 4}, 4, "Genesis", ts.Nodes(), true, nil)

	ts.IssueCandidacyAnnouncementInSlot("node1-candidacy:1", 4, "wave-1:4.3", ts.Wallet("node1"))
	ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:1", 5, "node1-candidacy:1", ts.Wallet("node4"))

	ts.IssueBlocksAtSlots("wave-2:", []iotago.SlotIndex{5, 6, 7, 8, 9}, 4, "node4-candidacy:1", ts.Nodes(), true, nil)

	ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:2", 9, "wave-2:9.3", ts.Wallet("node4"))
	ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:1", 9, "node4-candidacy:2", ts.Wallet("node5"))

	// This candidacy should be considered as it's announced at the last possible slot.
	ts.IssueCandidacyAnnouncementInSlot("node6-candidacy:1", 10, "node5-candidacy:1", ts.Wallet("node6"))

	ts.IssueBlocksAtSlots("wave-3:", []iotago.SlotIndex{10}, 4, "node6-candidacy:1", ts.Nodes(), true, nil)

	// Those candidacies should not be considered as they're issued after EpochNearingThreshold (slot 10).
	ts.IssueCandidacyAnnouncementInSlot("node2-candidacy:1", 11, "wave-3:10.3", ts.Wallet("node2"))
	ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:1", 11, "node2-candidacy:1", ts.Wallet("node3"))
	ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:3", 11, "node3-candidacy:1", ts.Wallet("node3"))
	ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:2", 11, "node4-candidacy:3", ts.Wallet("node3"))

	// Assert that only candidates that issued before slot 11 are considered.
	ts.AssertSybilProtectionCandidates(1, []iotago.AccountID{
		ts.Node("node1").Validator.AccountID,
		ts.Node("node4").Validator.AccountID,
		ts.Node("node5").Validator.AccountID,
		ts.Node("node6").Validator.AccountID,
	}, ts.Nodes()...)

	ts.IssueBlocksAtSlots("wave-4:", []iotago.SlotIndex{11, 12, 13, 14, 15, 16, 17}, 4, "node5-candidacy:2", ts.Nodes(), true, nil)

	ts.AssertLatestFinalizedSlot(13, ts.Nodes()...)
	ts.AssertSybilProtectionCommittee(1, []iotago.AccountID{
		ts.Node("node1").Validator.AccountID,
		ts.Node("node4").Validator.AccountID,
		ts.Node("node5").Validator.AccountID,
	}, ts.Nodes()...)
}
