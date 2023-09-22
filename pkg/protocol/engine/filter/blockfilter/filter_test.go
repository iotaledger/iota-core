package blockfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	apiForVersion, err := t.apiProvider.APIForVersion(block.ProtocolVersion)
	require.NoError(t.Test, err)

	modelBlock, err := model.BlockFromBlock(block, apiForVersion, serix.WithValidation())
	if err != nil {
		return err
	}

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, "")

	return nil
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) error {
	slot := t.apiProvider.CurrentAPI().TimeProvider().SlotFromTime(issuingTime)
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

func mockedCommitteeFunc(validatorAccountID iotago.AccountID) func(iotago.SlotIndex) *account.SeatedAccounts {
	mockedAccounts := account.NewAccounts()
	mockedAccounts.Set(validatorAccountID, new(account.Pool))
	seatedAccounts := account.NewSeatedAccounts(mockedAccounts)
	seatedAccounts.Set(account.SeatIndex(0), validatorAccountID)

	return func(slotIndex iotago.SlotIndex) *account.SeatedAccounts {
		return seatedAccounts
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
