package blockfilter

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

type TestFramework struct {
	Test   *testing.T
	Filter *Filter

	api iotago.API
}

func NewTestFramework(t *testing.T, api iotago.API, optsFilter ...options.Option[Filter]) *TestFramework {
	tf := &TestFramework{
		Test: t,
		api:  api,
	}
	tf.Filter = New(tf, optsFilter...)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		t.Logf("BlockPreAllowed: %s", block.ID())
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		t.Logf("BlockPreFiltered: %s - %s", event.Block.ID(), event.Reason)
	})

	return tf
}

func (t *TestFramework) APIForVersion(iotago.Version) iotago.API {
	return t.api
}

func (t *TestFramework) APIForSlot(_ iotago.SlotIndex) iotago.API {
	return t.api
}

func (t *TestFramework) APIForEpoch(_ iotago.EpochIndex) iotago.API {
	return t.api
}

func (t *TestFramework) LatestAPI() iotago.API {
	return t.api
}

func (t *TestFramework) processBlock(alias string, block *iotago.ProtocolBlock) {
	modelBlock, err := model.BlockFromBlock(block, t.api)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) {
	block, err := builder.NewBasicBlockBuilder(t.api).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlotWithPayload(alias string, slot iotago.SlotIndex, committing iotago.SlotIndex, payload iotago.Payload) {
	block, err := builder.NewBasicBlockBuilder(t.api).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(t.api.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(t.api.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
		Payload(payload).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlot(alias string, index iotago.SlotIndex, committing iotago.SlotIndex) {
	block, err := builder.NewBasicBlockBuilder(t.api).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(t.api.TimeProvider().SlotStartTime(index)).
		SlotCommitmentID(iotago.NewCommitment(t.api.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueSigned(alias string) {
	keyPair := ed25519.GenerateKeyPair()
	// We derive a dummy account from addr.
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBasicBlockBuilder(t.api).
		StrongParents(iotago.BlockIDs{}).
		IssuingTime(time.Now()).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func TestFilter_WithMaxAllowedWallClockDrift(t *testing.T) {
	allowedDrift := 3 * time.Second

	api := tpkg.TestAPI

	tf := NewTestFramework(t,
		api,
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

func TestFilter_MinCommittableAge(t *testing.T) {
	api := tpkg.TestAPI

	tf := NewTestFramework(t,
		api,
		WithMinCommittableAge(3),
	)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, ierrors.Is(event.Reason, ErrCommitmentTooRecent))
	})

	tf.IssueUnsignedBlockAtSlot("valid-1-0", 1, 0)
	tf.IssueUnsignedBlockAtSlot("valid-2-0", 2, 0)
	tf.IssueUnsignedBlockAtSlot("valid-3-0", 3, 0)
	tf.IssueUnsignedBlockAtSlot("valid-4-0", 4, 0)

	tf.IssueUnsignedBlockAtSlot("invalid-1-1", 1, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-2-1", 2, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-3-1", 3, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-10-1", 10, 1)
	tf.IssueUnsignedBlockAtSlot("valid-11-1", 11, 1)

	tf.IssueUnsignedBlockAtSlot("valid-5-0", 5, 0)
	tf.IssueUnsignedBlockAtSlot("invalid-4-1", 4, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-5-2", 5, 2)
	tf.IssueUnsignedBlockAtSlot("invalid-5-3", 5, 3)
	tf.IssueUnsignedBlockAtSlot("invalid-5-4", 5, 4)
	tf.IssueUnsignedBlockAtSlot("invalid-5-5", 5, 5)
	tf.IssueUnsignedBlockAtSlot("invalid-5-6", 5, 6)

	tf.IssueUnsignedBlockAtSlot("invalid-19-10", 19, 10)
	tf.IssueUnsignedBlockAtSlot("valid-19-9", 19, 9)
	tf.IssueUnsignedBlockAtSlot("valid-19-8", 19, 8)

}

func TestFilter_TransactionCommitmentInput(t *testing.T) {
	// with the following parameters, block issued in slot 110 can contain a transaction with commitment input referencing
	// commitments between 90 and slot that the block commits to (100 at most)
	api := iotago.V3API(
		iotago.NewV3ProtocolParameters(
			iotago.WithTimeProviderOptions(time.Now().Add(-20*time.Minute).Unix(), 10, 13),
			iotago.WithLivenessOptions(10, 20, 3, 4),
		),
	)

	tf := NewTestFramework(t,
		api,
	)

	tf.Filter.events.BlockPreAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, ierrors.Is(event.Reason, ErrInvalidProofOfWork))
	})

	commitmentInputTooOld, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(79, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputTooOld", 100, 80, commitmentInputTooOld)

	commitmentInputNewerThanBlockCommitment, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputNewerThanBlockCommitment", 100, 80, commitmentInputNewerThanBlockCommitment)

	commitmentCorrect, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrect)

	commitmentCorrectOldest, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectOldest", 100, 80, commitmentCorrectOldest)

	commitmentCorrectNewest, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(90, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrectNewest)

	commitmentCorrectMiddle, err := builder.NewTransactionBuilder(api).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner())

	require.NoError(tf.Test, err)

	tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectMiddle", 100, 90, commitmentCorrectMiddle)

}
