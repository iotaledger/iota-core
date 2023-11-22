package tests

import (
	"testing"
	"time"

	"github.com/iotaledger/iota-core/pkg/testsuite"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

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
	ts.Run(true)

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

	ts.AssertBlocksExist(ts.Blocks("block1", "block2"), false, ts.Nodes()...)
	ts.AssertBlocksInRetainerFailureReason(ts.Blocks("block1", "block2"), apimodels.BlockFailureInvalid, ts.Nodes()...)
}
