package blockfilter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
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
