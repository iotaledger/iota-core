package blockfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	Test   *testing.T
	Filter *Filter

	apiProvider iotago.APIProvider
}

func NewTestFramework(t *testing.T, apiProvider iotago.APIProvider, optsFilter ...options.Option[Filter]) *TestFramework {
	tf := &TestFramework{
		Test:        t,
		apiProvider: apiProvider,
	}
	tf.Filter = New(apiProvider, optsFilter...)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		t.Logf("BlockPreAllowed: %s", block.ID())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		t.Logf("BlockPreFiltered: %s - %s", event.Block.ID(), event.Reason)
	})

	return tf
}

func (t *TestFramework) processBlock(alias string, block *iotago.ProtocolBlock) error {
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	if err != nil {
		return err
	}

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, "")

	return nil
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) error {
	slot := t.apiProvider.CommittedAPI().TimeProvider().SlotFromTime(issuingTime)
	block, err := builder.NewBasicBlockBuilder(t.apiProvider.APIForSlot(slot)).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func (t *TestFramework) IssueValidationBlockAtTime(alias string, issuingTime time.Time, validatorAccountID iotago.AccountID) error {
	version := t.apiProvider.LatestAPI().ProtocolParameters().Version()
	block, err := builder.NewValidationBlockBuilder(t.apiProvider.LatestAPI()).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		HighestSupportedVersion(version).
		Sign(validatorAccountID, tpkg.RandEd25519PrivateKey()).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func (t *TestFramework) IssueBlockAtSlotWithVersion(alias string, slot iotago.SlotIndex, version iotago.Version, apiProvider iotago.APIProvider) error {
	apiForVersion, err := apiProvider.APIForVersion(version)
	require.NoError(t.Test, err)

	block, err := builder.NewBasicBlockBuilder(apiForVersion).
		ProtocolVersion(version).
		StrongParents(iotago.BlockIDs{iotago.BlockID{}}).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(apiForVersion.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(apiForVersion.Version(), slot-apiForVersion.ProtocolParameters().MinCommittableAge(), iotago.CommitmentID{}, iotago.Identifier{}, 0, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func mockedCommitteeFunc(validatorAccountID iotago.AccountID) func(iotago.SlotIndex) (*account.SeatedAccounts, bool) {
	mockedAccounts := account.NewAccounts()
	mockedAccounts.Set(validatorAccountID, new(account.Pool))
	seatedAccounts := account.NewSeatedAccounts(mockedAccounts)
	seatedAccounts.Set(account.SeatIndex(0), validatorAccountID)

	return func(slot iotago.SlotIndex) (*account.SeatedAccounts, bool) {
		return seatedAccounts, true
	}
}

func TestFilter_WithMaxAllowedWallClockDrift(t *testing.T) {
	allowedDrift := 3 * time.Second

	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.SingleVersionProvider(testAPI),
		WithMaxAllowedWallClockDrift(allowedDrift),
	)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.NotEqual(t, "tooFarAheadFuture", block.ID().Alias())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.Equal(t, "tooFarAheadFuture", event.Block.ID().Alias())
		require.True(t, ierrors.Is(event.Reason, ErrBlockTimeTooFarAheadInFuture))
	})

	require.NoError(t, tf.IssueUnsignedBlockAtTime("past", time.Now().Add(-allowedDrift)))
	require.NoError(t, tf.IssueUnsignedBlockAtTime("present", time.Now()))
	require.NoError(t, tf.IssueUnsignedBlockAtTime("acceptedFuture", time.Now().Add(allowedDrift)))
	require.NoError(t, tf.IssueUnsignedBlockAtTime("tooFarAheadFuture", time.Now().Add(allowedDrift).Add(1*time.Second)))
}

func TestFilter_ProtocolVersion(t *testing.T) {
	apiProvider := api.NewEpochBasedProvider(
		api.WithAPIForMissingVersionCallback(
			func(params iotago.ProtocolParameters) (iotago.API, error) {
				return iotago.V3API(iotago.NewV3ProtocolParameters(iotago.WithVersion(params.Version()))), nil
			},
		),
	)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithVersion(3)), 0)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithVersion(4)), 3)

	timeProvider := apiProvider.CommittedAPI().TimeProvider()

	tf := NewTestFramework(t,
		apiProvider,
		// Set this to some value far in the future so that we can arbitrarily manipulate block times.
		WithMaxAllowedWallClockDrift(time.Duration(uint64(timeProvider.EpochEnd(50))*uint64(timeProvider.SlotDurationSeconds()))*time.Second),
	)

	valid := ds.NewSet[string]()
	invalid := ds.NewSet[string]()

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.True(t, valid.Has(block.ID().Alias()))
		require.False(t, invalid.Has(block.ID().Alias()))
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		block := event.Block
		require.False(t, valid.Has(block.ID().Alias()))
		require.True(t, invalid.Has(block.ID().Alias()))
		require.True(t, ierrors.Is(event.Reason, ErrInvalidBlockVersion))
	})

	invalid.Add("A")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("A", timeProvider.EpochStart(1), 4, apiProvider))

	valid.Add("B")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("B", timeProvider.EpochEnd(1), 3, apiProvider))

	valid.Add("C")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("C", timeProvider.EpochEnd(2), 3, apiProvider))

	invalid.Add("D")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("D", timeProvider.EpochStart(3), 3, apiProvider))

	valid.Add("E")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("E", timeProvider.EpochStart(3), 4, apiProvider))
	valid.Add("F")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("F", timeProvider.EpochEnd(3), 4, apiProvider))

	valid.Add("G")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("G", timeProvider.EpochStart(5)+5, 4, apiProvider))

	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithVersion(5)), 10)

	valid.Add("H")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("H", timeProvider.EpochEnd(9), 4, apiProvider))

	invalid.Add("I")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("I", timeProvider.EpochStart(10), 4, apiProvider))

	valid.Add("J")
	require.NoError(t, tf.IssueBlockAtSlotWithVersion("J", timeProvider.EpochStart(10), 5, apiProvider))
}

func TestFilter_ValidationBlocks(t *testing.T) {
	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.SingleVersionProvider(testAPI),
	)

	validatorAccountID := tpkg.RandAccountID()
	nonValidatorAccountID := tpkg.RandAccountID()

	tf.Filter.committeeFunc = mockedCommitteeFunc(validatorAccountID)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.Equal(t, "validator", block.ID().Alias())
		require.NotEqual(t, "nonValidator", block.ID().Alias())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.NotEqual(t, "validator", event.Block.ID().Alias())
		require.Equal(t, "nonValidator", event.Block.ID().Alias())
		require.True(t, ierrors.Is(event.Reason, ErrValidatorNotInCommittee))
	})

	require.NoError(t, tf.IssueValidationBlockAtTime("validator", time.Now(), validatorAccountID))
	require.NoError(t, tf.IssueValidationBlockAtTime("nonValidator", time.Now(), nonValidatorAccountID))
}
