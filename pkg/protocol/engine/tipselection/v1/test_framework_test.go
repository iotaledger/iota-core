package tipselectionv1_test

import (
	"testing"
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
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

	optCommitteeSize     int
	optLivenessThreshold func(tipmanager.TipMetadata) time.Duration
}

func NewTestFramework(test *testing.T, opts ...options.Option[TestFramework]) *TestFramework {
	return options.Apply(&TestFramework{
		test:             test,
		createdBlocks:    make(map[iotago.BlockID]*blocks.Block),
		optCommitteeSize: 10,
		optLivenessThreshold: func(tipmanager.TipMetadata) time.Duration {
			// TODO: implement default
			return 0
		},
	}, opts, func(t *TestFramework) {
		transactionMetadataRetriever := func(iotago.TransactionID) (mempool.TransactionMetadata, bool) {
			return nil, false
		}

		rootBlocksRetriever := func() iotago.BlockIDs {
			return iotago.BlockIDs{iotago.EmptyBlockID()}
		}

		t.TipManager = tipmanagertests.NewTestFramework(test)

		t.Instance = tipselectionv1.New().Construct(
			t.TipManager.Instance,
			conflictdagv1.New[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank](t.CommitteeSize),
			transactionMetadataRetriever,
			rootBlocksRetriever,
			t.optLivenessThreshold,
		)
	})
}

func (t *TestFramework) CommitteeSize() int {
	return t.optCommitteeSize
}

func WithCommitteeSize(size int) options.Option[TestFramework] {
	return func(args *TestFramework) {
		args.optCommitteeSize = size
	}
}
