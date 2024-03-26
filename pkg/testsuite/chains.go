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

func (t *TestSuite) AssertCommitmentsOnChainAndChainHasCommitments(expectedCommitments []*model.Commitment, chainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			var selectedChain *protocol.Chain
			_ = node.Protocol.Chains.Set.ForEach(func(chain *protocol.Chain) error {
				if forkingPoint := chain.ForkingPoint.Get(); forkingPoint != nil && forkingPoint.ID() == chainID {
					selectedChain = chain
				}

				return nil
			})

			if chainID != iotago.EmptyCommitmentID && selectedChain == nil {
				return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: chain with forking point %s not found", node.Name, chainID)
			}

			for _, expectedCommitment := range expectedCommitments {
				// Check that passed commitments have the correct chain assigned.
				{
					protocolCommitment, err := node.Protocol.Commitments.Get(expectedCommitment.ID(), false)
					if err != nil {
						return ierrors.Wrapf(err, "AssertCommitmentsOnChainAndChainHasCommitments: %s: expected commitment %s on chain %s not found", node.Name, expectedCommitment.ID(), chainID)
					}

					if protocolCommitment.Chain.Get() != selectedChain {
						if selectedChain == nil {
							return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment %s not on correct chain, expected nil, got %s (pointer: %p, name: %s)", node.Name, expectedCommitment.ID(), protocolCommitment.Chain.Get().ForkingPoint.Get().ID(), protocolCommitment.Chain.Get(), protocolCommitment.Chain.Get().LogName())
						}

						return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment %s not on correct chain, expected %s (pointer: %p, name: %s), got %s (pointer: %p, name: %s)", node.Name, expectedCommitment.ID(), chainID, selectedChain, selectedChain.LogName(), protocolCommitment.Chain.Get().ForkingPoint.Get().ID(), protocolCommitment.Chain.Get(), protocolCommitment.Chain.Get().LogName())
					}
				}

				// Check that the chain has correct commitments assigned in its metadata.
				if selectedChain != nil {
					commitment, exists := selectedChain.Commitment(expectedCommitment.Slot())
					if !exists {
						return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment for slot %d does not exist on the selected chain %s", node.Name, expectedCommitment.Slot(), chainID)
					}

					if expectedCommitment.ID() != commitment.ID() {
						return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment on chain does not match, expected %s, got %s", node.Name, expectedCommitment, commitment.ID())
					}
				}
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCommitmentsOnChain(expectedCommitments []*model.Commitment, expectedChainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			for _, expectedCommitment := range expectedCommitments {
				// Check that passed commitments have the correct chain assigned.
				{
					protocolCommitment, err := node.Protocol.Commitments.Get(expectedCommitment.ID(), false)
					if err != nil {
						return ierrors.Wrapf(err, "AssertCommitmentsOnChainAndChainHasCommitments: %s: expected commitment %s on chain %s not found", node.Name, expectedCommitment.ID(), expectedChainID)
					}

					if protocolCommitment.Chain.Get() == nil {
						if expectedChainID != iotago.EmptyCommitmentID {
							return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment %s (name: %s) not on correct chain, expected %s, got nil", node.Name, expectedCommitment.ID(), protocolCommitment.LogName(), expectedChainID)
						}
					} else {
						if expectedChainID != protocolCommitment.Chain.Get().ForkingPoint.Get().ID() {
							return ierrors.Errorf("AssertCommitmentsOnChainAndChainHasCommitments: %s: commitment %s not on correct chain, expected %s, got %s (pointer: %p, name: %s)", node.Name, expectedCommitment.ID(), expectedChainID, protocolCommitment.Chain.Get().ForkingPoint.Get().ID(), protocolCommitment.Chain.Get(), protocolCommitment.Chain.Get().LogName())
						}
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

func (t *TestSuite) AssertCommitmentsAndChainsEvicted(expectedEvictedSlot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			seenChains := make(map[*protocol.Chain]struct{})
			if err := node.Protocol.Commitments.ForEach(func(commitment *protocol.Commitment) error {
				if commitment.Chain.Get() != nil { // the chain of orphaned commitments is nil.
					seenChains[commitment.Chain.Get()] = struct{}{}
				}

				if expectedEvictedSlot >= commitment.Slot() {
					return ierrors.Errorf("AssertCommitmentsAndChainsEvicted: %s: commitment %s not evicted", node.Name, commitment.ID())
				}

				return nil
			}); err != nil {
				return err
			}

			if err := node.Protocol.Chains.ForEach(func(chain *protocol.Chain) error {
				for i := iotago.SlotIndex(0); i <= expectedEvictedSlot; i++ {
					commitment, exists := chain.Commitment(expectedEvictedSlot)
					if exists {
						return ierrors.Errorf("AssertCommitmentsAndChainsEvicted: %s: commitment %s on chain %s not evicted", node.Name, commitment.ID(), chain.ForkingPoint.Get().ID())
					}
				}

				return nil
			}); err != nil {
				return err
			}

			// Make sure that we don't have dangling chains.
			if err := node.Protocol.Chains.Set.ForEach(func(chain *protocol.Chain) error {
				if _, exists := seenChains[chain]; !exists {
					return ierrors.Errorf("AssertCommitmentsAndChainsEvicted: %s: chain %s not evicted, total count of chains (from commitments)=%d, actual (in Protocol.Chains)=%d", node.Name, chain.ForkingPoint.Get().ID(), len(seenChains), node.Protocol.Chains.Set.Size())
				}

				return nil
			}); err != nil {
				return err
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCommitmentsOnEvictedChain(expectedCommitments []*model.Commitment, expectedOrphaned bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			for _, expectedCommitment := range expectedCommitments {
				commitment, err := node.Protocol.Commitments.Get(expectedCommitment.ID(), false)
				if err != nil {
					return ierrors.Wrapf(err, "AssertCommitmentsOnEvictedChain: %s: expected commitment %s not found", node.Name, expectedCommitment.ID())
				}

				if chain := commitment.Chain.Get(); expectedOrphaned != (chain == nil || chain.IsEvicted.Get()) {
					return ierrors.Errorf("AssertCommitmentsOnEvictedChain: %s: expected commitment %s to be on evicted chain %t, got %t", node.Name, expectedCommitment.ID(), expectedOrphaned, chain == nil || chain.IsEvicted.Get())
				}
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertMainChain(expectedChainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			mainChainID := node.Protocol.Chains.Main.Get().ForkingPoint.Get().ID()

			if mainChainID != expectedChainID {
				return ierrors.Errorf("AssertMainChain: %s: expected main chain to be %s, got %s", node.Name, expectedChainID, mainChainID)
			}

			return nil
		})
	}
}
