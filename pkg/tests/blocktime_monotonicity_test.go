package tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func Test_MaxAllowedWallClockDrift(t *testing.T) {
	allowedDrift := 3 * time.Second

	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(2, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
		),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	ts.Run(false, map[string][]options.Option[protocol.Protocol]{"node0": {
		protocol.WithMaxAllowedWallClockDrift(allowedDrift),
	}})

	pastBlock := lo.PanicOnErr(node0.Validator.CreateBasicBlock(context.Background(), "past", node0, mock.WithBasicBlockHeader(mock.WithIssuingTime(time.Now().Add(-allowedDrift)))))
	ts.RegisterBlock("past", pastBlock)
	require.NoError(t, node0.Validator.IssueBlock(pastBlock.ModelBlock(), node0))

	presentBlock := lo.PanicOnErr(node0.Validator.CreateBasicBlock(context.Background(), "present", node0, mock.WithBasicBlockHeader(mock.WithIssuingTime(time.Now()))))
	ts.RegisterBlock("present", presentBlock)
	require.NoError(t, node0.Validator.IssueBlock(presentBlock.ModelBlock(), node0))

	acceptedFutureBlock := lo.PanicOnErr(node0.Validator.CreateBasicBlock(context.Background(), "acceptedFuture", node0, mock.WithBasicBlockHeader(mock.WithIssuingTime(time.Now().Add(allowedDrift)))))
	ts.RegisterBlock("acceptedFuture", acceptedFutureBlock)
	require.NoError(t, node0.Validator.IssueBlock(acceptedFutureBlock.ModelBlock(), node0))

	tooFarAheadFutureBlock := lo.PanicOnErr(node0.Validator.CreateBasicBlock(context.Background(), "tooFarAheadFuture", node0, mock.WithBasicBlockHeader(mock.WithIssuingTime(time.Now().Add(allowedDrift).Add(1*time.Second)))))
	ts.RegisterBlock("tooFarAheadFuture", tooFarAheadFutureBlock)
	require.NoError(t, node0.Validator.IssueBlock(tooFarAheadFutureBlock.ModelBlock(), node0))

	ts.AssertBlocksExist(ts.Blocks("past", "present", "acceptedFuture"), true, node0)
	ts.AssertBlocksExist(ts.Blocks("tooFarAheadFuture"), false, node0)
}
func Test_BlockTimeMonotonicity(t *testing.T) {
	ts := testsuite.NewTestSuite(t,
		testsuite.WithProtocolParametersOptions(
			iotago.WithTimeProviderOptions(
				0,
				testsuite.GenesisTimeWithOffsetBySlots(100, testsuite.DefaultSlotDurationInSeconds),
				testsuite.DefaultSlotDurationInSeconds,
				testsuite.DefaultSlotsPerEpochExponent,
			),
		),
	)
	defer ts.Shutdown()

	node0 := ts.AddValidatorNode("node0")
	ts.Run(false)

	time0 := ts.API.TimeProvider().GenesisTime().Add(3 * time.Second)
	ts.IssueValidationBlockWithOptions("block0", node0, mock.WithValidationBlockHeaderOptions(
		mock.WithStrongParents(ts.BlockIDs("Genesis")...),
		mock.WithIssuingTime(time0),
	))

	ts.AssertBlocksExist(ts.Blocks("block0"), true, ts.Nodes()...)
	ts.AssertBlocksInCacheBooked(ts.Blocks("block0"), true, ts.Nodes()...)

	// Issue block1 with time0 -1 nanosecond
	{
		time1 := time0.Add(-1 * time.Nanosecond)
		ts.IssueValidationBlockWithOptions("block1", node0, mock.WithValidationBlockHeaderOptions(
			mock.WithStrongParents(ts.BlockIDs("Genesis", "block0")...),
			mock.WithIssuingTime(time1),
			mock.WithSkipReferenceValidation(true),
		))
	}

	// Issue block2 equal to time0
	{
		ts.IssueValidationBlockWithOptions("block2", node0, mock.WithValidationBlockHeaderOptions(
			mock.WithStrongParents(ts.BlockIDs("Genesis", "block0")...),
			mock.WithIssuingTime(time0),
			mock.WithSkipReferenceValidation(true),
		))

	}

	ts.AssertBlockFiltered(ts.Blocks("block1", "block2"), iotago.ErrBlockIssuingTimeNonMonotonic, node0)
}
