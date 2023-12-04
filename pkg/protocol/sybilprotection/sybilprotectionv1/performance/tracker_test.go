package performance

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestManager_Rewards(t *testing.T) {
	ts := NewTestSuite(t)
	// performance factor testing
	epoch := iotago.EpochIndex(2)
	epochActions := map[string]*EpochActions{
		"A": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 40, 40, 40, 20},
			FixedCost:                   10,
			ActiveSlotsCount:            8, // ideal performance
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
		"B": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 20, 10, 30, 80},
			FixedCost:                   10,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 6, // versus low performance, one block per subslot
			SlotPerformance:             6,
		},
		"C": {
			PoolStake:                   200,
			ValidatorStake:              40,
			Delegators:                  []iotago.BaseToken{20, 20, 10, 30, 80},
			FixedCost:                   10,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 10, // versus the same performance, many blocks in one subslot
			SlotPerformance:             4,
		},
	}
	ts.ApplyEpochActions(epoch, epochActions)
	ts.AssertEpochRewards(epoch, epochActions)
	// better performin validator should get more rewards
	ts.AssertValidatorRewardGreaterThan("A", "B", epoch, epochActions)

	epoch = iotago.EpochIndex(3)
	epochActions = map[string]*EpochActions{
		"A": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{2, 3},
			FixedCost:                   10,
			ActiveSlotsCount:            6, // validator dropped out for two last slots
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
		"C": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{3, 2},
			FixedCost:                   100,
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: uint64(ts.api.ProtocolParameters().ValidationBlocksPerSlot() + 2), // no reward for validator issuing more blocks than allowed
			SlotPerformance:             10,
		},
		"D": {
			PoolStake:                   10,
			ValidatorStake:              5,
			Delegators:                  []iotago.BaseToken{3, 2},
			FixedCost:                   100_000_000_000, // fixed cost higher than the pool reward, no reward for validator
			ActiveSlotsCount:            8,
			ValidationBlocksSentPerSlot: 10,
			SlotPerformance:             10,
		},
	}
	ts.ApplyEpochActions(epoch, epochActions)
	ts.AssertEpochRewards(epoch, epochActions)
	ts.AssertNoReward("C", epoch, epochActions)
	ts.AssertRewardForDelegatorsOnly("D", epoch, epochActions)

	// test the epoch after initial phase
}

func TestManager_Candidates(t *testing.T) {
	ts := NewTestSuite(t)

	issuer1 := tpkg.RandAccountID()
	issuer2 := tpkg.RandAccountID()
	issuer3 := tpkg.RandAccountID()
	{
		block1 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)

		block1.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(1)
		block1.Header.IssuerID = issuer1

		block2 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)

		block2.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(2)
		block2.Header.IssuerID = issuer2

		block3 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)

		block3.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(3)
		block3.Header.IssuerID = issuer3

		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block1))))
		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block2))))
		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block3))))
	}

	{
		block4 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)
		block4.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(4)
		block4.Header.IssuerID = issuer1

		block5 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)

		block5.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(5)
		block5.Header.IssuerID = issuer2

		block6 := tpkg.RandBlock(tpkg.RandBasicBlockBody(ts.api, iotago.PayloadCandidacyAnnouncement), ts.api, 0)

		block6.Header.IssuingTime = ts.api.TimeProvider().SlotStartTime(6)
		block6.Header.IssuerID = issuer3

		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block4))))
		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block5))))
		ts.Instance.TrackCandidateBlock(blocks.NewBlock(lo.PanicOnErr(model.BlockFromBlock(block6))))
	}

	require.True(t, lo.PanicOnErr(ts.Instance.EligibleValidatorCandidates(0)).HasAll(ds.NewReadableSet(issuer1, issuer2, issuer3)))
	require.True(t, lo.PanicOnErr(ts.Instance.ValidatorCandidates(0)).HasAll(ds.NewReadableSet(issuer1, issuer2, issuer3)))
	require.True(t, lo.PanicOnErr(ts.Instance.EligibleValidatorCandidates(1)).IsEmpty())
	require.True(t, lo.PanicOnErr(ts.Instance.ValidatorCandidates(1)).IsEmpty())
}
