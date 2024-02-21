package testsuite

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertLatestEngineCommitmentOnMainChain(nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			chain := node.Protocol.Chains.Main.Get()
			if chain == nil {
				return ierrors.Errorf("AssertLatestEngineCommitmentOnMainChain: %s: chain is nil", node.Name)
			}

			latestChainCommitment := chain.LatestCommitment.Get()
			latestCommitment := node.Protocol.Engines.Main.Get().SyncManager.LatestCommitment()

			if latestCommitment.ID() != latestChainCommitment.ID() {
				return ierrors.Errorf("AssertLatestEngineCommitmentOnMainChain: %s: latest commitment is not equal, expected %s, got %s", node.Name, latestCommitment.ID(), latestChainCommitment.ID())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCommitmentsOnChain(expectedCommitments []*model.Commitment, chainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			var selectedChain *protocol.Chain
			_ = node.Protocol.Chains.Set.ForEach(func(chain *protocol.Chain) error {
				if chain.ForkingPoint.Get().ID() == chainID {
					selectedChain = chain
				}

				return nil
			})

			if chainID != iotago.EmptyCommitmentID && selectedChain == nil {
				return ierrors.Errorf("AssertCommitmentsOnChain: %s: chain with forking point %s not found", node.Name, chainID)
			}

			for _, expectedCommitment := range expectedCommitments {
				// Check that passed commitments have the correct chain assigned.
				{
					expectedCommitment, err := node.Protocol.Commitments.Get(expectedCommitment.ID(), false)
					if err != nil {
						return ierrors.Wrapf(err, "AssertCommitmentsOnChain: %s: expected commitment %s on chain %s not found", node.Name, expectedCommitment, chainID)
					}

					if expectedCommitment.Chain.Get() != selectedChain {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment %s not on correct chain, expected %s, got %s", node.Name, expectedCommitment, chainID, expectedCommitment.Chain.Get().ForkingPoint.Get().ID())
					}
				}

				// Check that the chain has correct commitments assigned in its metadata.
				if selectedChain != nil {
					commitment, exists := selectedChain.Commitment(expectedCommitment.Slot())
					if !exists {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment for slot %d does not exist on the selected chain %s", node.Name, expectedCommitment.Slot(), chainID)
					}

					if expectedCommitment.ID() != commitment.ID() {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment on chain does not match, expected %s, got %s", node.Name, expectedCommitment, commitment.ID())
					}
				}
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertUniqueCommitmentChain(nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			commitmentCountPerChain := shrinkingmap.New[*protocol.Chain, *shrinkingmap.ShrinkingMap[iotago.SlotIndex, []iotago.CommitmentID]]()
			_ = node.Protocol.Commitments.ForEach(func(commitment *protocol.Commitment) error {
				// Orphaned commitments have chain set to nil, we want to ignore them in this check.
				if commitment.Chain.Get() == nil {
					return nil
				}

				commitmentCountForChain, _ := commitmentCountPerChain.GetOrCreate(commitment.Chain.Get(), func() *shrinkingmap.ShrinkingMap[iotago.SlotIndex, []iotago.CommitmentID] {
					return shrinkingmap.New[iotago.SlotIndex, []iotago.CommitmentID]()
				})

				commitmentCountForChain.Compute(commitment.Slot(), func(currentValue []iotago.CommitmentID, _ bool) []iotago.CommitmentID {
					return append(currentValue, commitment.ID())
				})

				return nil
			})

			incorrectCommitments := make(map[iotago.CommitmentID][]iotago.CommitmentID)
			commitmentCountPerChain.ForEach(func(chain *protocol.Chain, commitmentCountForChain *shrinkingmap.ShrinkingMap[iotago.SlotIndex, []iotago.CommitmentID]) bool {
				for _, commitments := range commitmentCountForChain.Values() {
					if len(commitments) > 1 {
						incorrectCommitments[chain.ForkingPoint.Get().ID()] = append(incorrectCommitments[chain.ForkingPoint.Get().ID()], commitments...)
					}
				}

				return true
			})

			if len(incorrectCommitments) > 0 {
				return ierrors.Errorf("AssertUniqueCommitmentChain: %s: multiple commitments for a slot use the same chain, %s", node.Name, incorrectCommitments)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCommitmentsOrphaned(expectedCommitments []*model.Commitment, expectedOrphaned bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			for _, expectedCommitment := range expectedCommitments {
				commitment, err := node.Protocol.Commitments.Get(expectedCommitment.ID(), false)
				if err != nil {
					return ierrors.Wrapf(err, "AssertCommitmentsOrphaned: %s: expected commitment %s not found", node.Name, expectedCommitment.ID())
				}

				if expectedOrphaned != commitment.IsOrphaned.Get() {
					return ierrors.Errorf("AssertCommitmentsOrphaned: %s: expected commitment %s to be orphaned %t, got %t", node.Name, expectedCommitment.ID(), expectedOrphaned, commitment.IsOrphaned.Get())
				}
			}

			return nil
		})
	}
}
