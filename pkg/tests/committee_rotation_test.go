package tests

import (
	"testing"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_TopStakersRotation(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(1000, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				4,
			),
			iotago.WithLivenessOptions(
				10,
				10,
				2,
				4,
				5,
			),
			iotago.WithTargetCommitteeSize(3),
		),
	)
	defer ts.Shutdown()

	node1 := ts.AddValidatorNode("node1", testsuite.WithWalletAmount(1_000_006))
	ts.AddValidatorNode("node2", testsuite.WithWalletAmount(1_000_005))
	ts.AddValidatorNode("node3", testsuite.WithWalletAmount(1_000_004))
	ts.AddValidatorNode("node4", testsuite.WithWalletAmount(1_000_003))
	ts.AddValidatorNode("node5", testsuite.WithWalletAmount(1_000_002))
	ts.AddValidatorNode("node6", testsuite.WithWalletAmount(1_000_001))
	ts.AddDefaultWallet(node1)

	ts.AddNode("node7")

	nodeOpts := []options.Option[protocol.Protocol]{
		protocol.WithNotarizationProvider(
			slotnotarization.NewProvider(),
		),
		protocol.WithSybilProtectionProvider(
			sybilprotectionv1.NewProvider(
				sybilprotectionv1.WithSeatManagerProvider(
					topstakers.NewProvider(),
				),
			),
		),
	}

	ts.Run(true, map[string][]options.Option[protocol.Protocol]{
		"node1": nodeOpts,
		"node2": nodeOpts,
		"node3": nodeOpts,
		"node4": nodeOpts,
		"node5": nodeOpts,
		"node6": nodeOpts,
		"node7": nodeOpts,
	})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		ts.Node("node1").Validator.AccountData.ID,
		ts.Node("node2").Validator.AccountData.ID,
		ts.Node("node3").Validator.AccountData.ID,
	}, ts.Nodes()...)

	// Select committee for epoch 1 and test candidacy announcements at different times.
	{
		ts.IssueBlocksAtSlots("wave-1:", []iotago.SlotIndex{1, 2, 3, 4}, 4, "Genesis", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node1-candidacy:1", 4, "wave-1:4.3", ts.Wallet("node1"))
		ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:1", 5, "node1-candidacy:1", ts.Wallet("node4"))

		ts.IssueBlocksAtSlots("wave-2:", []iotago.SlotIndex{5, 6, 7, 8, 9}, 4, "node4-candidacy:1", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:2", 9, "wave-2:9.3", ts.Wallet("node4"))
		ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:1", 9, "node4-candidacy:2", ts.Wallet("node5"))

		// This candidacy should be considered as it's announced at the last possible slot.
		ts.IssueCandidacyAnnouncementInSlot("node6-candidacy:1", 10, "node5-candidacy:1", ts.Wallet("node6"))

		ts.IssueBlocksAtSlots("wave-3:", []iotago.SlotIndex{10}, 4, "node6-candidacy:1", ts.Nodes(), true, false)

		// Those candidacies should not be considered as they're issued after EpochNearingThreshold (slot 10).
		ts.IssueCandidacyAnnouncementInSlot("node2-candidacy:1", 11, "wave-3:10.3", ts.Wallet("node2"))
		ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:1", 11, "node2-candidacy:1", ts.Wallet("node3"))
		ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:3", 11, "node3-candidacy:1", ts.Wallet("node3"))
		ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:2", 11, "node4-candidacy:3", ts.Wallet("node3"))

		// Assert that only candidates that issued before slot 11 are considered.
		ts.AssertSybilProtectionCandidates(0, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
			ts.Node("node6").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.IssueBlocksAtSlots("wave-4:", []iotago.SlotIndex{11, 12, 13, 14, 15, 16, 17}, 4, "node5-candidacy:2", ts.Nodes(), true, false)

		ts.AssertLatestFinalizedSlot(14, ts.Nodes()...)
		ts.AssertSybilProtectionCommittee(1, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertReelectedCommitteeSeatIndices(0, 1, ts.Nodes()...)
	}

	// Do not announce new candidacies for epoch 2 but finalize slots. The committee should be the reused.
	{
		ts.IssueBlocksAtSlots("wave-5:", []iotago.SlotIndex{18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30}, 4, "wave-4:17.3", ts.Nodes(), true, false)

		ts.AssertSybilProtectionCandidates(1, []iotago.AccountID{}, ts.Nodes()...)
		ts.AssertLatestCommitmentSlotIndex(28, ts.Nodes()...)
		ts.AssertLatestFinalizedSlot(27, ts.Nodes()...)
		ts.AssertSybilProtectionCommittee(2, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertReelectedCommitteeSeatIndices(1, 2, ts.Nodes()...)
	}

	// Do not finalize slots in time for epoch 3. The committee should be the reused. Even though there are candidates.
	{
		// Issue blocks to remove the inactive committee members.
		ts.IssueBlocksAtSlots("wave-6:", []iotago.SlotIndex{31, 32}, 4, "wave-5:30.3", ts.Nodes("node5", "node7"), false, false)
		ts.AssertLatestCommitmentSlotIndex(30, ts.Nodes()...)

		ts.IssueCandidacyAnnouncementInSlot("node6-candidacy:2", 33, "wave-6:32.3", ts.Wallet("node6"))

		// Issue the rest of the epoch just before we reach epoch end - maxCommittableAge.
		ts.IssueBlocksAtSlots("wave-7:", []iotago.SlotIndex{33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45}, 4, "node6-candidacy:2", ts.Nodes("node5"), true, false)

		ts.AssertLatestCommitmentSlotIndex(43, ts.Nodes()...)
		// Even though we have a candidate, the committee should be reused as we did not finalize at epochNearingThreshold before epoch end - maxCommittableAge was committed
		ts.AssertSybilProtectionCandidates(2, []iotago.AccountID{
			ts.Node("node6").Validator.AccountData.ID,
		}, ts.Nodes()...)
		// Check that the committee is reused.
		ts.AssertSybilProtectionCommittee(3, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertReelectedCommitteeSeatIndices(2, 3, ts.Nodes()...)
	}

	// Rotate committee to smaller committee due to too few candidates available.
	// node3 - SeatIndex(0) as it's the only one
	{
		ts.IssueBlocksAtSlots("wave-8:", []iotago.SlotIndex{46, 47, 48, 49, 50, 51, 52, 53, 54, 55}, 4, "wave-7:45.3", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:2", 56, "wave-8:55.3", ts.Wallet("node3"))

		ts.IssueBlocksAtSlots("wave-8:", []iotago.SlotIndex{56, 57, 58, 59, 60, 61}, 4, "node3-candidacy:2", ts.Nodes(), true, false)

		ts.AssertLatestCommitmentSlotIndex(59, ts.Nodes()...)
		ts.AssertLatestFinalizedSlot(58, ts.Nodes()...)
		// We finalized at epochEnd-epochNearingThreshold, so the committee should be rotated even if there is just one candidate.
		ts.AssertSybilProtectionCandidates(3, []iotago.AccountID{
			ts.Node("node3").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertSybilProtectionCommittee(4, []iotago.AccountID{
			ts.Node("node3").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertReelectedCommitteeSeatIndices(3, 4, ts.Nodes()...)
	}

	// Rotate committee to bigger committee.
	// node3 - SeatIndex(0) - seat should be retained
	// node5 - SeatIndex(1)
	{
		ts.IssueBlocksAtSlots("wave-9:", []iotago.SlotIndex{62, 63, 64, 65, 66, 67, 68, 69, 70}, 4, "wave-8:61.3", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:3", 71, "wave-9:70.3", ts.Wallet("node3"))
		ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:3", 71, "node3-candidacy:3", ts.Wallet("node5"))

		ts.IssueBlocksAtSlots("wave-9:", []iotago.SlotIndex{71, 72, 73, 74, 75, 76, 77, 78, 80}, 4, "node5-candidacy:3", ts.Nodes(), true, false)

		ts.AssertLatestCommitmentSlotIndex(78, ts.Nodes()...)
		ts.AssertLatestFinalizedSlot(76, ts.Nodes()...)
		// We finalized at epochEnd-epochNearingThreshold, so the committee should be rotated even if there is just one candidate.
		ts.AssertSybilProtectionCandidates(4, []iotago.AccountID{
			ts.Node("node3").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertSybilProtectionCommittee(5, []iotago.AccountID{
			ts.Node("node3").Validator.AccountData.ID,
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertReelectedCommitteeSeatIndices(4, 5, ts.Nodes()...)
	}

	// Rotate committee to smaller committee, where the committee member is re-selected to make sure that the SeatIndex is retained.
	// Node5 - SeatIndex(1) - it's the only committee member and needs to retain its seat index.
	{
		ts.IssueBlocksAtSlots("wave-10:", []iotago.SlotIndex{81, 82, 83, 84, 85, 86, 87}, 4, "wave-9:80.3", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node5-candidacy:4", 88, "wave-10:87.3", ts.Wallet("node5"))

		ts.IssueBlocksAtSlots("wave-10:", []iotago.SlotIndex{88, 89, 90, 91, 92, 93, 94}, 4, "node5-candidacy:4", ts.Nodes(), true, false)
		ts.AssertLatestCommitmentSlotIndex(92, ts.Nodes()...)
		ts.AssertLatestFinalizedSlot(91, ts.Nodes()...)
		// We finalized at epochEnd-epochNearingThreshold, so the committee should be rotated even if there is just one candidate.
		ts.AssertSybilProtectionCandidates(5, []iotago.AccountID{
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertSybilProtectionCommittee(6, []iotago.AccountID{
			ts.Node("node5").Validator.AccountData.ID,
		}, ts.Nodes()...)
		ts.AssertReelectedCommitteeSeatIndices(5, 6, ts.Nodes()...)
	}
}
