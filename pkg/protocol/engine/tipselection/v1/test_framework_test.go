package tipselectionv1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag/spenddagv1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	tipmanagertests "github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager/tests"
	tipselectionv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection/v1"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance   *tipselectionv1.TipSelection
	TipManager *tipmanagertests.TestFramework

	test          *testing.T
	createdBlocks map[iotago.BlockID]*blocks.Block

	expectedLivenessDuration func(tip tipmanager.TipMetadata) time.Duration

	optCommitteeSize int
}

func NewTestFramework(test *testing.T, opts ...options.Option[TestFramework]) *TestFramework {
	return options.Apply(&TestFramework{
		test:             test,
		createdBlocks:    make(map[iotago.BlockID]*blocks.Block),
		optCommitteeSize: 10,
	}, opts, func(t *TestFramework) {
		t.expectedLivenessDuration = tipselectionv1.DynamicLivenessThreshold(func() int { return t.optCommitteeSize })

		transactionMetadataRetriever := func(iotago.TransactionID) (mempool.TransactionMetadata, bool) {
			return nil, false
		}

		rootBlockRetriever := func() iotago.BlockID {
			return iotago.EmptyBlockID
		}

		t.TipManager = tipmanagertests.NewTestFramework(test)

		t.Instance = tipselectionv1.New(module.NewTestModule(test)).Construct(
			t.TipManager.Instance,
			spenddagv1.New[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank](t.CommitteeSize),
			transactionMetadataRetriever,
			rootBlockRetriever,
			t.expectedLivenessDuration,
		)
	})
}

func (t *TestFramework) LowerLivenessThreshold(alias string) time.Time {
	block := t.TipManager.Block(alias)

	return block.IssuingTime().Add(block.ProtocolBlock().API.ProtocolParameters().LivenessThresholdLowerBound())
}

func (t *TestFramework) UpperLivenessThreshold(alias string) time.Time {
	block := t.TipManager.Block(alias)

	return block.IssuingTime().Add(block.ProtocolBlock().API.ProtocolParameters().LivenessThresholdUpperBound())
}

func (t *TestFramework) ExpectedLivenessThreshold(alias string) time.Time {
	tipMetadata := t.TipManager.TipMetadata(alias)

	return tipMetadata.Block().IssuingTime().Add(t.expectedLivenessDuration(tipMetadata))
}

func (t *TestFramework) RequireLivenessThreshold(alias string, expectedDuration time.Duration) {
	tipMetadata := t.TipManager.TipMetadata(alias)

	require.Equal(t.test, expectedDuration, t.expectedLivenessDuration(tipMetadata))
}

func (t *TestFramework) CommitteeSize() int {
	return t.optCommitteeSize
}

func WithCommitteeSize(size int) options.Option[TestFramework] {
	return func(args *TestFramework) {
		args.optCommitteeSize = size
	}
}
