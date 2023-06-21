package blockfilter

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

type TestFramework struct {
	Test   *testing.T
	Filter *Filter
	api    iotago.API
}

func NewTestFramework(t *testing.T, protocolParams *iotago.ProtocolParameters, blockIssuerCheck func(*iotago.Block) bool, optsFilter ...options.Option[Filter]) *TestFramework {
	tf := &TestFramework{
		Test: t,
		api:  iotago.V3API(protocolParams),

		Filter: New(func() *iotago.ProtocolParameters {
			return protocolParams
		}, blockIssuerCheck, optsFilter...),
	}

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		t.Logf("BlockAllowed: %s", block.ID())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		t.Logf("BlockFiltered: %s - %s", event.Block.ID(), event.Reason)
	})

	return tf
}

func (t *TestFramework) processBlock(alias string, block *iotago.Block) {
	modelBlock, err := model.BlockFromBlock(block, t.api)
	require.NoError(t.Test, err)

	modelBlock.ID().RegisterAlias(alias)
	t.Filter.ProcessReceivedBlock(modelBlock, network.PeerID{})
}

func (t *TestFramework) IssueUnsignedBlockAtTime(alias string, issuingTime time.Time) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(issuingTime).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockWithoutPoW(alias string) (score float64) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		Build()
	require.NoError(t.Test, err)

	score, _, err = block.POW()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)

	return score
}

func (t *TestFramework) IssueUnsignedBlockWithPoWScore(alias string, minPowScore float64) (score float64) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		ProofOfWork(context.Background(), minPowScore).
		Build()
	require.NoError(t.Test, err)

	score, _, err = block.POW()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)

	return score
}

func (t *TestFramework) IssueUnsignedBlockAtSlot(alias string, index iotago.SlotIndex, committing iotago.SlotIndex) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(t.api.SlotTimeProvider().StartTime(index)).
		SlotCommitment(iotago.NewCommitment(committing, iotago.CommitmentID{}, iotago.Identifier{}, 0)).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueUnsignedBlockWithBurnedMana(alias string, burnedMana uint64) {
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		BurnedMana(burnedMana).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

func (t *TestFramework) IssueSigned(alias string) {
	keyPair := ed25519.GenerateKeyPair()
	// We derive a dummy account from addr.
	addr := iotago.Ed25519AddressFromPubKey(keyPair.PublicKey[:])
	block, err := builder.NewBlockBuilder().
		StrongParents(iotago.StrongParentsIDs{iotago.BlockID{}}).
		IssuingTime(time.Now()).
		Sign(iotago.AccountID(addr[:]), keyPair.PrivateKey[:]).
		Build()
	require.NoError(t.Test, err)

	t.processBlock(alias, block)
}

var protoParams = iotago.ProtocolParameters{
	Version:     3,
	NetworkName: "test",
	Bech32HRP:   "rms",
	MinPoWScore: 0,
	RentStructure: iotago.RentStructure{
		VByteCost:    100,
		VBFactorKey:  10,
		VBFactorData: 1,
	},
	TokenSupply:           5000,
	GenesisUnixTimestamp:  uint32(time.Now().Unix()),
	SlotDurationInSeconds: 10,
	MaxCommitableAge:      10,
}

func TestFilter_WithMaxAllowedWallClockDrift(t *testing.T) {
	allowedDrift := 3 * time.Second

	tf := NewTestFramework(t,
		&protoParams,
		func(*iotago.Block) bool { return true },
		WithMaxAllowedWallClockDrift(allowedDrift),
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.NotEqual(t, "tooFarAheadFuture", block.ID().Alias())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.Equal(t, "tooFarAheadFuture", event.Block.ID().Alias())
		require.True(t, errors.Is(event.Reason, ErrBlockTimeTooFarAheadInFuture))
	})

	tf.IssueUnsignedBlockAtTime("past", time.Now().Add(-allowedDrift))
	tf.IssueUnsignedBlockAtTime("present", time.Now())
	tf.IssueUnsignedBlockAtTime("acceptedFuture", time.Now().Add(allowedDrift))
	tf.IssueUnsignedBlockAtTime("tooFarAheadFuture", time.Now().Add(allowedDrift).Add(1*time.Second))
}

func TestFilter_WithSignatureValidation(t *testing.T) {
	tf := NewTestFramework(t,
		&protoParams,
		func(*iotago.Block) bool { return true },
		WithSignatureValidation(true),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.Equal(t, "valid", block.ID().Alias())
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.Equal(t, "invalid", event.Block.ID().Alias())
		require.True(t, errors.Is(event.Reason, ErrInvalidSignature))
	})

	tf.IssueUnsignedBlockAtTime("invalid", time.Now())
	tf.IssueSigned("valid")
}

func TestFilter_MinCommittableSlotAge(t *testing.T) {
	params := protoParams
	params.GenesisUnixTimestamp = uint32(time.Now().Add(-5 * time.Minute).Unix())

	tf := NewTestFramework(t,
		&params,
		func(*iotago.Block) bool { return true },
		WithMinCommittableSlotAge(3),
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, errors.Is(event.Reason, ErrCommitmentNotCommittable))
	})

	tf.IssueUnsignedBlockAtSlot("valid-1-0", 1, 0)
	tf.IssueUnsignedBlockAtSlot("valid-2-0", 2, 0)
	tf.IssueUnsignedBlockAtSlot("valid-3-0", 3, 0)
	tf.IssueUnsignedBlockAtSlot("valid-4-0", 4, 0)

	tf.IssueUnsignedBlockAtSlot("invalid-1-1", 1, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-2-1", 2, 1)
	tf.IssueUnsignedBlockAtSlot("invalid-3-1", 3, 1)
	tf.IssueUnsignedBlockAtSlot("valid-4-1", 4, 1)
	tf.IssueUnsignedBlockAtSlot("valid-5-1", 5, 1)

	tf.IssueUnsignedBlockAtSlot("valid-5-0", 5, 0)
	tf.IssueUnsignedBlockAtSlot("valid-4-1", 5, 1)
	tf.IssueUnsignedBlockAtSlot("valid-5-2", 5, 2)
	tf.IssueUnsignedBlockAtSlot("invalid-5-3", 5, 3)
	tf.IssueUnsignedBlockAtSlot("invalid-5-4", 5, 4)
	tf.IssueUnsignedBlockAtSlot("invalid-5-5", 5, 5)
	tf.IssueUnsignedBlockAtSlot("invalid-5-6", 5, 6)
}

func TestFilter_MinPoW(t *testing.T) {
	params := protoParams
	params.MinPoWScore = 1000

	tf := NewTestFramework(t,
		&params,
		func(*iotago.Block) bool { return true },
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, errors.Is(event.Reason, ErrInvalidProofOfWork))
	})

	require.GreaterOrEqual(t, tf.IssueUnsignedBlockWithPoWScore("valid", 1000), float64(params.MinPoWScore))
	require.Less(t, tf.IssueUnsignedBlockWithoutPoW("invalid"), float64(params.MinPoWScore))
	require.GreaterOrEqual(t, tf.IssueUnsignedBlockWithPoWScore("valid", 1000), float64(params.MinPoWScore))
}

func TestFilter_BurnedMana(t *testing.T) {
	params := protoParams

	tf := NewTestFramework(t,
		&params,
		func(*iotago.Block) bool { return true },
		WithSignatureValidation(false),
		WithReferenceManaCost(5),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, errors.Is(event.Reason, ErrInsufficientBurnedMana))
	})

	tf.IssueUnsignedBlockWithBurnedMana("valid-5", 5)
	tf.IssueUnsignedBlockWithBurnedMana("valid-6", 6)

	tf.IssueUnsignedBlockWithBurnedMana("invalid-4", 4)
	tf.IssueUnsignedBlockWithBurnedMana("invalid-0", 0)

}

func TestFilter_BICCheck(t *testing.T) {
	params := protoParams

	// use a check that burned Mana is even number as a proxy for account BIC
	bicCheckFunc := func(b *iotago.Block) bool {
		return b.BurnedMana%2 == 0
	}

	tf := NewTestFramework(t,
		&params,
		bicCheckFunc,
		WithSignatureValidation(false),
	)

	tf.Filter.events.BlockAllowed.Hook(func(block *model.Block) {
		require.True(t, strings.HasPrefix(block.ID().Alias(), "valid"))
	})

	tf.Filter.events.BlockFiltered.Hook(func(event *filter.BlockFilteredEvent) {
		require.True(t, strings.HasPrefix(event.Block.ID().Alias(), "invalid"))
		require.True(t, errors.Is(event.Reason, ErrNegativeBIC))
	})

	tf.IssueUnsignedBlockWithBurnedMana("valid-0", 0)
	tf.IssueUnsignedBlockWithBurnedMana("valid-2", 2)

	tf.IssueUnsignedBlockWithBurnedMana("invalid-1", 1)
	tf.IssueUnsignedBlockWithBurnedMana("invalid-3", 3)

}
