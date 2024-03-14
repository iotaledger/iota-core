package requesthandler_test

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization/slotnotarization"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager/topstakers"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/sybilprotectionv1"
	"github.com/iotaledger/iota-core/pkg/requesthandler"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func Test_ValidatorsAPI(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(200, testsuite.DefaultSlotDurationInSeconds),
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
	})

	ts.AssertSybilProtectionCommittee(0, []iotago.AccountID{
		ts.Node("node1").Validator.AccountData.ID,
		ts.Node("node2").Validator.AccountData.ID,
		ts.Node("node3").Validator.AccountData.ID,
	}, ts.Nodes()...)

	requestHandler := node1.RequestHandler
	hrp := ts.API.ProtocolParameters().Bech32HRP()

	// Epoch 0, assert that node1 and node4 are the only candidates.
	{
		ts.IssueBlocksAtSlots("wave-1:", []iotago.SlotIndex{1, 2, 3, 4}, 4, "Genesis", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node1-candidacy:1", 4, "wave-1:4.3", ts.Wallet("node1"))
		ts.IssueCandidacyAnnouncementInSlot("node4-candidacy:1", 5, "node1-candidacy:1", ts.Wallet("node4"))

		ts.IssueBlocksAtSlots("wave-2:", []iotago.SlotIndex{5, 6, 7, 8}, 4, "node4-candidacy:1", ts.Nodes(), true, false)

		ts.AssertSybilProtectionCandidates(0, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertSybilProtectionRegisteredValidators(0, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, ts.Nodes()...)

		assertValidatorsFromRequestHandler(t, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, requestHandler, 0)
	}

	// Epoch 0, assert that node1, node2 and node4 are the only candidates.
	{
		ts.IssueCandidacyAnnouncementInSlot("node2-candidacy:1", 9, "wave-2:8.3", ts.Wallet("node2"))
		ts.IssueBlocksAtSlots("wave-3:", []iotago.SlotIndex{9, 10}, 4, "node2-candidacy:1", ts.Nodes(), true, false)

		ts.AssertSybilProtectionCandidates(0, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node2").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertSybilProtectionRegisteredValidators(0, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, ts.Nodes()...)

		assertValidatorsFromRequestHandler(t, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, requestHandler, 0)
	}

	// Epoch 0, assert that node1, node2 and node4 are the only registered validators. Since node3 issued a candidacy payload after epoch nearing threshold (slot 11), it should not be a registered validators.
	{
		ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:1", 11, "wave-3:10.3", ts.Wallet("node3"))
		ts.IssueBlocksAtSlots("wave-5:", []iotago.SlotIndex{11}, 4, "node3-candidacy:1", ts.Nodes(), true, false)

		ts.AssertSybilProtectionCandidates(0, []iotago.AccountID{
			ts.Node("node1").Validator.AccountData.ID,
			ts.Node("node2").Validator.AccountData.ID,
			ts.Node("node4").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertSybilProtectionRegisteredValidators(0, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, ts.Nodes()...)

		assertValidatorsFromRequestHandler(t, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, requestHandler, 0)
	}

	// Epoch 1, assert that node1, node2 and node4 are the only registered validators in Epoch 0. And node2 and node3 are the only registered validators in Epoch 1.
	{
		ts.IssueBlocksAtSlots("wave-6:", []iotago.SlotIndex{12, 13, 14, 15, 16}, 4, "wave-5:11.3", ts.Nodes(), true, false)

		ts.IssueCandidacyAnnouncementInSlot("node2-candidacy:2", 17, "wave-6:16.3", ts.Wallet("node2"))
		ts.IssueCandidacyAnnouncementInSlot("node3-candidacy:2", 17, "node2-candidacy:2", ts.Wallet("node3"))
		ts.IssueBlocksAtSlots("wave-7:", []iotago.SlotIndex{18}, 4, "node3-candidacy:2", ts.Nodes(), true, false)

		// request registered validators of epoch 0, the validator cache should be used.
		assertValidatorsFromRequestHandler(t, []string{
			ts.Node("node1").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node4").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, requestHandler, 0)

		// epoch 1 should have 2 validators (node2, node3)
		ts.AssertSybilProtectionCandidates(1, []iotago.AccountID{
			ts.Node("node2").Validator.AccountData.ID,
			ts.Node("node3").Validator.AccountData.ID,
		}, ts.Nodes()...)

		ts.AssertSybilProtectionRegisteredValidators(1, []string{
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node3").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, ts.Nodes()...)

		assertValidatorsFromRequestHandler(t, []string{
			ts.Node("node2").Validator.AccountData.ID.ToAddress().Bech32(hrp),
			ts.Node("node3").Validator.AccountData.ID.ToAddress().Bech32(hrp),
		}, requestHandler, 1)
	}

	// error returned, requesting with invalid cursor index.
	{
		_, err := requestHandler.Validators(1, 6, 10)
		require.ErrorAs(t, echo.ErrBadRequest, &err)
	}
}

func assertValidatorsFromRequestHandler(t *testing.T, expectedValidators []string, requestHandler *requesthandler.RequestHandler, requestedEpoch iotago.EpochIndex) {
	resp, err := requestHandler.Validators(requestedEpoch, 0, 10)
	require.NoError(t, err)
	actualValidators := lo.Map(resp.Validators, func(validator *api.ValidatorResponse) string {
		return validator.AddressBech32
	})

	require.ElementsMatch(t, expectedValidators, actualValidators)
}
