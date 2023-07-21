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
	"github.com/iotaledger/iota-core/pkg/core/api"
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

	apiProvider api.Provider
}

func NewTestFramework(t *testing.T, apiProvider api.Provider, optsFilter ...options.Option[Filter]) *TestFramework {
	tf := &TestFramework{
		Test:        t,
		apiProvider: apiProvider,
	}
	tf.Filter = New(apiProvider, optsFilter...)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		t.Logf("BlockAllowed: %s", block.ID())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		t.Logf("BlockFiltered: %s - %s", event.Block.ID(), event.Reason)
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
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
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
		SlotCommitmentID(iotago.NewCommitment(apiForSlot.Version(), committing, iotago.CommitmentID{}, iotago.Identifier{}, 0).MustID()).
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
		Build()
	require.NoError(t.Test, err)

	t.processBlockWithAPI(alias, block, api)
	return block
}

func TestFilter_ProtocolVersion(t *testing.T) {
	apiProvider := api.NewDynamicMockAPIProvider()
	apiProvider.AddProtocolParameters(0, newMockProtocolParameters(3))
	apiProvider.AddProtocolParameters(3, newMockProtocolParameters(4))

	defaultAPI := apiProvider.CurrentAPI()
	timeProvider := apiProvider.CurrentAPI().TimeProvider()

	tf := NewTestFramework(t,
		apiProvider,
		WithSignatureValidation(false),
		// Set this to some value far in the future so that we can arbitrarily manipulate block times.
		WithMaxAllowedWallClockDrift(time.Duration(uint64(timeProvider.EpochEnd(50))*uint64(timeProvider.SlotDurationSeconds()))*time.Second),
	)

	valid := ds.NewSet[string]()
	invalid := ds.NewSet[string]()

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, valid.Has(block.ID().Alias()))
		require.False(t, invalid.Has(block.ID().Alias()))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
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

	apiProvider.AddProtocolParameters(10, newMockProtocolParameters(5))

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
		api.NewStaticProvider(testAPI),
		WithMaxAllowedWallClockDrift(allowedDrift),
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.NotEqual(t, "tooFarAheadFuture", block.ID().Alias())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.Equal(t, "tooFarAheadFuture", event.Block.ID().Alias())
		require.True(t, ierrors.Is(event.Reason, ErrBlockTimeTooFarAheadInFuture))
	})

	tf.IssueUnsignedBlockAtTime("past", time.Now().Add(-allowedDrift))
	tf.IssueUnsignedBlockAtTime("present", time.Now())
	tf.IssueUnsignedBlockAtTime("acceptedFuture", time.Now().Add(allowedDrift))
	tf.IssueUnsignedBlockAtTime("tooFarAheadFuture", time.Now().Add(allowedDrift).Add(1*time.Second))
}

func TestFilter_WithSignatureValidation(t *testing.T) {
	testAPI := tpkg.TestAPI

	tf := NewTestFramework(t,
		api.NewStaticProvider(testAPI),
		WithSignatureValidation(true),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.Equal(t, "valid", block.ID().Alias())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.Contains(t, []string{"incorrectSignature", "pubkeysMissing"}, event.Block.ID().Alias())
		require.True(t, ierrors.Is(event.Reason, ErrInvalidSignature))
	})

	tf.IssueUnsignedBlockAtTime("pubkeysMissing", time.Now())
	tf.IssueSigned("valid")

	block, err := builder.NewBasicBlockBuilder(testAPI).
		StrongParents(iotago.BlockIDs{}).
		Build()
	require.NoError(tf.Test, err)

	block.Signature.(*iotago.Ed25519Signature).Signature = tpkg.Rand64ByteArray()
	block.Signature.(*iotago.Ed25519Signature).PublicKey = tpkg.Rand32ByteArray()

	tf.processBlock("incorrectSignature", block)
}

func TestFilter_ExpiryThreshold(t *testing.T) {
	v3API := iotago.V3API(
		iotago.NewV3ProtocolParameters(
			iotago.WithTimeProviderOptions(time.Now().Add(-5*time.Minute).Unix(), 10, 13),
		),
	)

	tf := NewTestFramework(t,
		api.NewStaticProvider(v3API),
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, ierrors.Is(event.Reason, ErrCommitmentNotCommittable))
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
	v3API := iotago.V3API(
		iotago.NewV3ProtocolParameters(
			iotago.WithTimeProviderOptions(time.Now().Add(-20*time.Minute).Unix(), 10, 13),
			iotago.WithLivenessOptions(10, 3, 4),
		),
	)

	tf := NewTestFramework(t,
		api.NewStaticProvider(v3API),
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.NotContains(t, []string{"commitmentInputTooOld", "commitmentInputNewerThanBlockCommitment"}, block.ID().Alias())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.Contains(t, []string{"commitmentInputTooOld", "commitmentInputNewerThanBlockCommitment"}, event.Block.ID().Alias())

		if strings.Contains(event.Block.ID().Alias(), "commitmentInputTooOld") {
			require.True(t, ierrors.Is(event.Reason, ErrTransactionCommitmentInputTooFarInThePast))
		}
		if strings.Contains(event.Block.ID().Alias(), "commitmentInputNewerThanBlockCommitment") {
			require.True(t, ierrors.Is(event.Reason, ErrTransactionCommitmentInputInTheFuture))
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

func newMockProtocolParameters(version iotago.Version) iotago.ProtocolParameters {
	return &mockProtocolParameters{version: version}
}

type mockProtocolParameters struct {
	version iotago.Version
}

func (m mockProtocolParameters) Version() iotago.Version {
	return m.version
}

func (m mockProtocolParameters) NetworkName() string {
	panic("implement me")
}

func (m mockProtocolParameters) NetworkID() iotago.NetworkID {
	panic("implement me")
}

func (m mockProtocolParameters) Bech32HRP() iotago.NetworkPrefix {
	panic("implement me")
}

func (m mockProtocolParameters) RentStructure() *iotago.RentStructure {
	panic("implement me")
}

func (m mockProtocolParameters) TokenSupply() iotago.BaseToken {
	panic("implement me")
}

func (m mockProtocolParameters) TimeProvider() *iotago.TimeProvider {
	return iotago.NewTimeProvider(0, 10, 13)
}

func (m mockProtocolParameters) ManaDecayProvider() *iotago.ManaDecayProvider {
	return iotago.NewManaDecayProvider(m.TimeProvider(), 0, 0, 0, nil, 0, 0, 0)
}

func (m mockProtocolParameters) StakingUnbondingPeriod() iotago.EpochIndex {
	panic("implement me")
}

func (m mockProtocolParameters) LivenessThreshold() iotago.SlotIndex {
	panic("implement me")
}

func (m mockProtocolParameters) LivenessThresholdDuration() time.Duration {
	panic("implement me")
}

func (m mockProtocolParameters) EvictionAge() iotago.SlotIndex {
	panic("implement me")
}

func (m mockProtocolParameters) EpochNearingThreshold() iotago.SlotIndex {
	panic("implement me")
}

func (m mockProtocolParameters) VersionSignaling() *iotago.VersionSignaling {
	panic("implement me")
}

func (m mockProtocolParameters) Bytes() ([]byte, error) {
	panic("implement me")
}

func (m mockProtocolParameters) Hash() (iotago.Identifier, error) {
	panic("implement me")
}
