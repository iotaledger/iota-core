package blockfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type TestFramework struct {
	Test   *testing.T
	Filter *Filter
	api    iotago.API
}

func NewTestFramework(t *testing.T, api iotago.API, optsFilter ...options.Option[Filter]) *TestFramework {
	tf := &TestFramework{
		Test: t,
		api:  api,

		Filter: New(optsFilter...),
	}

	tf.Filter.Events().BlockAllowed.Hook(func(block *model.Block) {
		t.Logf("BlockAllowed: %s", block.ID)
	})

	tf.Filter.Events().BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		t.Logf("BlockFiltered: %s - %s", event.Block.ID, event.Reason)
	})

	return tf
}

func (t *TestFramework) processBlock(alias string, block *iotago.Block) {
	modelBlock, err := model.BlockFromBlock(block, t.api)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, identity.NewID(ed25519.PublicKey{}))
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockAtSlot(alias string, index iotago.SlotIndex, committing iotago.SlotIndex) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(t.api.SlotTimeProvider().StartTime(index)).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueSigned(alias string) {
	keyPair := ed25519.GenerateKeyPair()
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(time.Now()).
		Sign(&addr, keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

// TODO: enable once filters are enabled again

// func TestFilter_WithMaxAllowedWallClockDrift(t *testing.T) {
// 	allowedDrift := 3 * time.Second
//
// 	tf := NewTestFramework(t,
// 		slot.NewTimeProvider(time.Now().Unix(), 10),
// 		WithMaxAllowedWallClockDrift(allowedDrift),
// 		WithSignatureValidation(false),
// 	)
//
// 	tf.Filter.Events().BlockAllowed.Hook(func(block *models.Block) {
// 		require.NotEqual(t, "tooFarAheadFuture", block.ID().Alias())
// 	})
//
// 	tf.Filter.Events().BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
// 		require.Equal(t, "tooFarAheadFuture", event.Block.ID().Alias())
// 		require.True(t, errors.Is(event.Reason, ErrorsBlockTimeTooFarAheadInFuture))
// 	})
//
// 	tf.IssueUnsignedBlockAtTime("past", time.Now().Add(-allowedDrift))
// 	tf.IssueUnsignedBlockAtTime("present", time.Now())
// 	tf.IssueUnsignedBlockAtTime("acceptedFuture", time.Now().Add(allowedDrift))
// 	tf.IssueUnsignedBlockAtTime("tooFarAheadFuture", time.Now().Add(allowedDrift).Add(1*time.Second))
// }
//
// func TestFilter_WithSignatureValidation(t *testing.T) {
// 	tf := NewTestFramework(t,
// 		slot.NewTimeProvider(time.Now().Unix(), 10),
// 		WithSignatureValidation(true),
// 	)
//
// 	tf.Filter.Events().BlockAllowed.Hook(func(block *models.Block) {
// 		require.Equal(t, "valid", block.ID().Alias())
// 	})
//
// 	tf.Filter.Events().BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
// 		require.Equal(t, "invalid", event.Block.ID().Alias())
// 		require.True(t, errors.Is(event.Reason, ErrorsInvalidSignature))
// 	})
//
// 	tf.IssueUnsignedBlockAtTime("invalid", time.Now())
// 	tf.IssueSigned("valid")
// }
//
// func TestFilter_MinCommittableSlotAge(t *testing.T) {
// 	tf := NewTestFramework(t,
// 		slot.NewTimeProvider(time.Now().Add(-5*time.Minute).Unix(), 10),
// 		WithMinCommittableSlotAge(3),
// 		WithSignatureValidation(false),
// 	)
//
// 	tf.Filter.Events().BlockAllowed.Hook(func(block *models.Block) {
// 		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
// 	})
//
// 	tf.Filter.Events().BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
// 		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
// 		require.True(t, errors.Is(event.Reason, ErrorCommitmentNotCommittable))
// 	})
//
// 	tf.IssueUnsignedBlockAtSlot("valid-1-0", 1, 0)
// 	tf.IssueUnsignedBlockAtSlot("valid-2-0", 2, 0)
// 	tf.IssueUnsignedBlockAtSlot("valid-3-0", 3, 0)
// 	tf.IssueUnsignedBlockAtSlot("valid-4-0", 4, 0)
//
// 	tf.IssueUnsignedBlockAtSlot("invalid-1-1", 1, 1)
// 	tf.IssueUnsignedBlockAtSlot("invalid-2-1", 2, 1)
// 	tf.IssueUnsignedBlockAtSlot("invalid-3-1", 3, 1)
// 	tf.IssueUnsignedBlockAtSlot("valid-4-1", 4, 1)
// 	tf.IssueUnsignedBlockAtSlot("valid-5-1", 5, 1)
//
// 	tf.IssueUnsignedBlockAtSlot("valid-5-0", 5, 0)
// 	tf.IssueUnsignedBlockAtSlot("valid-4-1", 5, 1)
// 	tf.IssueUnsignedBlockAtSlot("valid-5-2", 5, 2)
// 	tf.IssueUnsignedBlockAtSlot("invalid-5-3", 5, 3)
// 	tf.IssueUnsignedBlockAtSlot("invalid-5-4", 5, 4)
// 	tf.IssueUnsignedBlockAtSlot("invalid-5-5", 5, 5)
// 	tf.IssueUnsignedBlockAtSlot("invalid-5-6", 5, 6)
// }
