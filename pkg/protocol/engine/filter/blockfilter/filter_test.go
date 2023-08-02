package blockfilter

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	Test   *testing.T
	Filter *Filter

	apiProvider api.Provider
}

func NewTestFramework(t *testing.T, apiProvider api.Provider, optsFilter ...options.Option[Filter]) *TestFramework {
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

func (t *TestFramework) processBlock(alias string, block *iotago.ProtocolBlock) {
	apiForVersion, err := t.apiProvider.APIForVersion(block.ProtocolVersion)
	require.NoError(t.Test, err)

	modelBlock, err := model.BlockFromBlock(block, apiForVersion)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})
}

func (t *TestFramework) processBlockWithAPI(alias string, block *iotago.ProtocolBlock, api iotago.API) {
	modelBlock, err := model.BlockFromBlock(block, api)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) {
	slot := t.apiProvider.CurrentAPI().TimeProvider().SlotFromTime(issuingTime)
	block, err := builder.NewBasicBlockBuilder(t.apiProvider.APIForSlot(slot)).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlotWithPayload(alias string, slot iotago.SlotIndex, committing iotago.SlotIndex, payload iotago.Payload) {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0, 0).MustID()).
		Payload(payload).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlot(alias string, slot iotago.SlotIndex, committing iotago.SlotIndex) {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueSigned(alias string) {
	keyPair := ed25519.GenerateKeyPair()
	// We derive a dummy account from addr.
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBasicBlockBuilder(t.apiProvider.CurrentAPI()).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(time.Now()).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueBlockAtSlotWithVersion(alias string, index iotago.SlotIndex, version iotago.Version, api iotago.API) *iotago.ProtocolBlock {
	block, err := builder.NewBasicBlockBuilder(api).
		ProtocolVersion(version).
		StrongParents(iotago.BlockIDs{iotago.BlockID{}}).
		IssuingTime(api.TimeProvider().SlotStartTime(index)).
		SlotCommitmentID(iotago.NewCommitment(api.Version(), index-api.ProtocolParameters().MinCommittableAge(), iotago.CommitmentID{}, iotago.Identifier{}, 0, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	t.processBlockWithAPI(alias, block, api)
	return block
}

func TestFilter_ProtocolVersion(t *testing.T) {
	apiProvider := api.NewEpochBasedProvider(
		api.WithAPIForMissingVersionCallback(
			func(version iotago.Version) (iotago.API, error) {
				return iotago.V3API(iotago.NewV3ProtocolParameters(iotago.WithVersion(version))), nil
			},
		),
	)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(), 0)
	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithVersion(4)), 3)

	defaultAPI := apiProvider.CurrentAPI()
	timeProvider := apiProvider.CurrentAPI().TimeProvider()

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
	tf.IssueBlockAtSlotWithVersion("A", timeProvider.EpochStart(1), 2, defaultAPI)

	valid.Add("B")
	tf.IssueBlockAtSlotWithVersion("B", timeProvider.EpochEnd(1), 3, defaultAPI)

	valid.Add("C")
	tf.IssueBlockAtSlotWithVersion("C", timeProvider.EpochEnd(2), 3, defaultAPI)

	invalid.Add("D")
	tf.IssueBlockAtSlotWithVersion("D", timeProvider.EpochStart(3), 3, defaultAPI)

	valid.Add("E")
	tf.IssueBlockAtSlotWithVersion("E", timeProvider.EpochStart(3), 4, defaultAPI)
	valid.Add("F")
	tf.IssueBlockAtSlotWithVersion("F", timeProvider.EpochEnd(3), 4, defaultAPI)

	valid.Add("G")
	tf.IssueBlockAtSlotWithVersion("G", timeProvider.EpochStart(5), 4, defaultAPI)

	apiProvider.AddProtocolParametersAtEpoch(iotago.NewV3ProtocolParameters(iotago.WithVersion(5)), 10)

	valid.Add("H")
	tf.IssueBlockAtSlotWithVersion("H", timeProvider.EpochEnd(9), 4, defaultAPI)

	invalid.Add("I")
	tf.IssueBlockAtSlotWithVersion("I", timeProvider.EpochStart(10), 4, defaultAPI)

	valid.Add("J")
	tf.IssueBlockAtSlotWithVersion("J", timeProvider.EpochStart(10), 5, defaultAPI)
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

	tf.IssueUnsignedBlockAtTime("past", time.Now().Add(-allowedDrift))
	tf.IssueUnsignedBlockAtTime("present", time.Now())
	tf.IssueUnsignedBlockAtTime("acceptedFuture", time.Now().Add(allowedDrift))
	tf.IssueUnsignedBlockAtTime("tooFarAheadFuture", time.Now().Add(allowedDrift).Add(1*time.Second))
}

func TestFilter_Commitments(t *testing.T) {
	// with the following parameters, a block issued in slot 100 can commit between slot 80 and 90
	v3API := iotago.V3API(
		iotago.NewV3ProtocolParameters(
			iotago.WithTimeProviderOptions(time.Now().Add(-20*time.Minute).Unix(), 10, 13),
			iotago.WithLivenessOptions(3, 10, 20, 4),
		),
	)

	tf := NewTestFramework(t,
		api.SingleVersionProvider(v3API),
	)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.NotContains(t, []string{"commitmentInputTooOld", "commitmentInputNewerThanBlockCommitment", "commitmentInputTooRecent"}, block.ID().Alias())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.Contains(t, []string{"commitmentTooOld", "commitmentTooRecent"}, event.Block.ID().Alias())

		if strings.Contains(event.Block.ID().Alias(), "commitmentInputTooOld") {
			require.True(t, ierrors.Is(event.Reason, ErrCommitmentInputTooOld))
		}
		if strings.Contains(event.Block.ID().Alias(), "commitmentInputTooRecent") {
			require.True(t, ierrors.Is(event.Reason, ErrCommitmentInputTooRecent))
		}
	})

	tf.IssueUnsignedBlockAtSlot("commitmentTooOld", 100, 79)

	tf.IssueUnsignedBlockAtSlot("commitmentTooRecent", 100, 91)

	tf.IssueUnsignedBlockAtSlot("commitmentCorrectNewest", 100, 90)

	tf.IssueUnsignedBlockAtSlot("commitmentCorrectOldest", 100, 80)

	tf.IssueUnsignedBlockAtSlot("commitmentCorrectMiddle", 100, 85)
}

func TestFilter_TransactionCommitmentInput(t *testing.T) {
	// with the following parameters, block issued in slot 110 can contain a transaction with commitment input referencing
	// commitments between 90 and slot that the block commits to (100 at most)
	v3API := iotago.V3API(
		iotago.NewV3ProtocolParameters(
			iotago.WithTimeProviderOptions(time.Now().Add(-20*time.Minute).Unix(), 10, 13),
			iotago.WithLivenessOptions(3, 10, 20, 4),
		),
	)

	tf := NewTestFramework(t,
		api.SingleVersionProvider(v3API),
	)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.NotContains(t, []string{"commitmentInputTooOld", "commitmentInputNewerThanBlockCommitment"}, block.ID().Alias())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.Contains(t, []string{"commitmentInputTooOld", "commitmentInputNewerThanBlockCommitment"}, event.Block.ID().Alias())

		if strings.Contains(event.Block.ID().Alias(), "commitmentInputTooOld") {
			require.True(t, ierrors.Is(event.Reason, ErrCommitmentInputTooOld))
		}
		if strings.Contains(event.Block.ID().Alias(), "commitmentInputNewerThanBlockCommitment") {
			require.True(t, ierrors.Is(event.Reason, ErrCommitmentInputNewerThanCommitment))
		}
	})

	commitmentInputTooOld, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(79, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputTooOld", 100, 80, commitmentInputTooOld)

	commitmentInputNewerThanBlockCommitment, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputNewerThanBlockCommitment", 100, 80, commitmentInputNewerThanBlockCommitment)

	commitmentCorrect, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrect)

	commitmentCorrectOldest, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectOldest", 100, 80, commitmentCorrectOldest)

	commitmentCorrectNewest, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(90, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrectNewest)

	commitmentCorrectMiddle, err := builder.NewTransactionBuilder(v3API).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectMiddle", 100, 90, commitmentCorrectMiddle)

}
