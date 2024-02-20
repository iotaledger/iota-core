package testsuite

import (
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
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

func (t *TestSuite) AssertCommitmentsOnChain(expectedCommitments []iotago.CommitmentID, chainID iotago.CommitmentID, nodes ...*mock.Node) {
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

			if selectedChain == nil {
				return ierrors.Errorf("AssertCommitmentsOnChain: %s: chain with forking point %s not found", node.Name, chainID)
			}

			for _, expectedCommitmentID := range expectedCommitments {
				// Check that passed commitments have the correct chain assigned.
				{
					expectedCommitment, err := node.Protocol.Commitments.Get(expectedCommitmentID, false)
					if err != nil {
						return ierrors.Wrapf(err, "AssertCommitmentsOnChain: %s: expected commitment %s on chain %s not found", node.Name, expectedCommitmentID, chainID)
					}

					if expectedCommitment.Chain.Get() != selectedChain {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment %s not on correct chain, expected %s, got %s", node.Name, expectedCommitmentID, chainID, expectedCommitment.Chain.Get().ForkingPoint.Get().ID())
					}
				}

				// Check that the chain has correct commitments assigned in its metadata.
				{
					commitment, exists := selectedChain.Commitment(expectedCommitmentID.Slot())
					if !exists {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment for slot %d does not exist on the selected chain %s", node.Name, expectedCommitmentID.Slot(), chainID)
					}

					if expectedCommitmentID != commitment.ID() {
						return ierrors.Errorf("AssertCommitmentsOnChain: %s: commitment on chain does not match, expected %s, got %s", node.Name, expectedCommitmentID, commitment.ID())
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

//func (t *TestSuite) AssertCommitmentsOrphaned(commitments []iotago.CommitmentID, expectedOrphaned bool, nodes ...*mock.Node) {
//	mustNodes(nodes)
//
//	for _, node := range nodes {
//		t.Eventually(func() error {
//
//			return nil
//		})
//	}
//}
