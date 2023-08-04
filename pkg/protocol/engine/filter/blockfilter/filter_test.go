package blockfilter

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/utils"
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

func (t *TestFramework) processBlock(alias string, block *iotago.ProtocolBlock) error {
	apiForVersion, err := t.apiProvider.APIForVersion(block.ProtocolVersion)
	require.NoError(t.Test, err)

	modelBlock, err := model.BlockFromBlock(block, apiForVersion, serix.WithValidation())
	if err != nil {
		return err
	}

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})

	return nil
}

func (t *TestFramework) processBlockWithAPI(alias string, block *iotago.ProtocolBlock, api iotago.API) {
	modelBlock, err := model.BlockFromBlock(block, api, serix.WithValidation())
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})
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

func (t *TestFramework) IssueUnsignedBlockAtSlotWithPayload(alias string, slot iotago.SlotIndex, committing iotago.SlotIndex, payload iotago.Payload) error {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
		Payload(payload).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlot(alias string, slot iotago.SlotIndex, committing iotago.SlotIndex) error {
	apiForSlot := t.apiProvider.APIForSlot(slot)

	block, err := builder.NewBasicBlockBuilder(apiForSlot).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(apiForSlot.TimeProvider().SlotStartTime(slot)).
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func (t *TestFramework) IssueSigned(alias string) error {
	keyPair := hiveEd25519.GenerateKeyPair()
	// We derive a dummy account from addr.
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBasicBlockBuilder(t.apiProvider.CurrentAPI()).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(time.Now()).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	return t.processBlock(alias, block)
}

func (t *TestFramework) IssueBlockAtSlotWithVersion(alias string, index iotago.SlotIndex, version iotago.Version, api iotago.API) *iotago.ProtocolBlock {
	block, err := builder.NewBasicBlockBuilder(api).
		ProtocolVersion(version).
		StrongParents(iotago.BlockIDs{tpkg.RandBlockID()}).
		IssuingTime(api.TimeProvider().SlotStartTime(index)).
		SlotCommitmentID(iotago.NewCommitment(api.Version(), index-api.ProtocolParameters().MinCommittableAge(), iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
		Build()
	require.NoError(t.Test, err)

	t.processBlockWithAPI(alias, block, api)
	return block
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
		require.Fail(t, "no blocks should be pre-filtered in this test")
	})

	require.ErrorIs(t, tf.IssueUnsignedBlockAtSlot("commitmentTooOld", 100, 79), iotago.ErrCommitmentTooOld)

	require.ErrorIs(t, tf.IssueUnsignedBlockAtSlot("commitmentTooRecent", 100, 91), iotago.ErrCommitmentTooRecent)

	require.NoError(t, tf.IssueUnsignedBlockAtSlot("commitmentCorrectNewest", 100, 90))

	require.NoError(t, tf.IssueUnsignedBlockAtSlot("commitmentCorrectOldest", 100, 80))

	require.NoError(t, tf.IssueUnsignedBlockAtSlot("commitmentCorrectMiddle", 100, 85))
}

func TestFilter_TransactionCommitmentInput(t *testing.T) {
	keyPair := hiveEd25519.GenerateKeyPair()
	// We derive a dummy account from addr.
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])

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
		require.Fail(t, "no blocks should be pre-filtered in this test")
	})

	commitmentInputTooOld, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(79, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.ErrorIs(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputTooOld", 100, 80, commitmentInputTooOld), iotago.ErrCommitmentInputTooOld)

	commitmentInputNewerThanBlockCommitment, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.ErrorIs(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentInputNewerThanBlockCommitment", 100, 80, commitmentInputNewerThanBlockCommitment), iotago.ErrCommitmentInputNewerThanCommitment)

	commitmentCorrect, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.NoError(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrect))

	commitmentCorrectOldest, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(80, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.NoError(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectOldest", 100, 80, commitmentCorrectOldest))

	commitmentCorrectNewest, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(90, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.NoError(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectNewest", 100, 90, commitmentCorrectNewest))

	commitmentCorrectMiddle, err := builder.NewTransactionBuilder(v3API).
		AddInput(&builder.TxInput{
			UnlockTarget: addr,
			InputID:      tpkg.RandOutputID(0),
			Input:        utils.RandOutput(iotago.OutputBasic),
		}).
		AddOutput(utils.RandOutput(iotago.OutputBasic)).
		AddContextInput(&iotago.CommitmentInput{CommitmentID: iotago.NewSlotIdentifier(85, tpkg.Rand32ByteArray())}).
		Build(iotago.NewInMemoryAddressSigner(iotago.AddressKeys{Address: addr, Keys: ed25519.PrivateKey(keyPair.PrivateKey[:])}))

	require.NoError(tf.Test, err)

	require.NoError(t, tf.IssueUnsignedBlockAtSlotWithPayload("commitmentCorrectMiddle", 100, 90, commitmentCorrectMiddle))

}
