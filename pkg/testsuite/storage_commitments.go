package testsuite

import (
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageCommitments(commitments []*iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, commitment := range commitments {
			t.Eventually(func() error {
				storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitment.Slot)
				if err != nil {
					return ierrors.Wrapf(err, "AssertStorageCommitments: %s: error loading commitment: %s", node.Name, commitment.MustID())
				}

				if !assert.Equal(t.fakeTesting, *commitment, *storedCommitment.Commitment()) {
					return ierrors.Errorf("AssertStorageCommitments: %s: expected %s, got %s", node.Name, commitment, storedCommitment)
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) AssertEqualStoredCommitmentAtIndex(index iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	t.Eventually(func() error {
		var commitment *model.Commitment
		var commitmentNode *mock.Node
		for _, node := range nodes {
			storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(index)
			if err != nil {
				return ierrors.Wrapf(err, "AssertEqualStoredCommitmentAtIndex: %s: error loading commitment for slot: %d", node.Name, index)
			}

			if commitment == nil {
				commitment = storedCommitment
				commitmentNode = node

				continue
			}

			if !assert.Equal(t.fakeTesting, *commitment.Commitment(), *storedCommitment.Commitment()) {
				return ierrors.Errorf("AssertEqualStoredCommitmentAtIndex: %s: expected %s (from %s), got %s", node.Name, commitment, commitmentNode.Name, storedCommitment)
			}
		}

		return nil
	})
}

func (t *TestSuite) AssertStorageCommitmentBlocks(slot iotago.SlotIndex, expectedBlocksBySlotCommitmentIDs map[iotago.CommitmentID]iotago.BlockIDs, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error loading commitment for slot: %d", node.Name, slot)
			}

			committedSlot, err := node.Protocol.Engines.Main.Get().CommitmentAPI(storedCommitment.ID())
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error getting committed slot for commitment: %s", node.Name, storedCommitment.ID())
			}

			committedBlocksBySlotCommitmentIDs, err := committedSlot.BlocksIDsBySlotCommitmentID()
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error getting committed blocks for slot: %d", node.Name, slot)
			}

			if len(committedBlocksBySlotCommitmentIDs) == 0 {
				committedBlocksBySlotCommitmentIDs = nil
			}

			if len(expectedBlocksBySlotCommitmentIDs) == 0 {
				expectedBlocksBySlotCommitmentIDs = nil
			}

			if len(committedBlocksBySlotCommitmentIDs) != len(expectedBlocksBySlotCommitmentIDs) {
				return ierrors.Errorf("AssertStorageCommitmentBlocks: %s: commitment count does not match - expected %s, got %s", node.Name, expectedBlocksBySlotCommitmentIDs, committedBlocksBySlotCommitmentIDs)
			}

			for expectedBlocksBySlotCommitmentID, expectedBlockIDs := range expectedBlocksBySlotCommitmentIDs {
				committedBlockIDs, ok := committedBlocksBySlotCommitmentIDs[expectedBlocksBySlotCommitmentID]
				if !ok {
					return ierrors.Errorf("AssertStorageCommitmentBlocks: %s: %s not found in committedBlockIDs - expected %s, got %s", node.Name, expectedBlocksBySlotCommitmentID, expectedBlocksBySlotCommitmentIDs, committedBlocksBySlotCommitmentIDs)
				}

				if !assert.ElementsMatch(t.fakeTesting, expectedBlockIDs, committedBlockIDs) {
					return ierrors.Errorf("AssertStorageCommitmentBlocks: %s: for commitment %s: expected %s, got %s", node.Name, expectedBlocksBySlotCommitmentID, expectedBlockIDs, committedBlockIDs)
				}
			}

			return nil
		})
	}
}

// AssertStorageCommitmentBlockAccepted asserts that the given block ID has the expected containment status in the commitment identified by the given slot.
func (t *TestSuite) AssertStorageCommitmentBlockAccepted(slot iotago.SlotIndex, blockID iotago.BlockID, expectedContains bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlockAccepted: %s: error loading commitment for slot: %d", node.Name, slot)
			}

			committedSlot, err := node.Protocol.Engines.Main.Get().CommitmentAPI(storedCommitment.ID())
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlockAccepted: %s: error getting committed slot for commitment: %s", node.Name, storedCommitment.ID())
			}

			acceptedBlocksBySlotCommitment, err := committedSlot.BlocksIDsBySlotCommitmentID()
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlockAccepted: %s: error getting BlocksIDsBySlotCommitmentID for commitment: %s", node.Name, storedCommitment.ID())
			}

			// Check if the Block ID is anywhere in the map.
			var actualContains bool
			for _, blockIDs := range acceptedBlocksBySlotCommitment {
				for _, innerBlockID := range blockIDs {
					if innerBlockID == blockID {
						actualContains = true
						break
					}
				}
			}

			if actualContains != expectedContains {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlockAccepted: %s: commitment %s's actual and expected containment of block id %s do not match; actual contained status: %t, expected: %t", node.Name, storedCommitment.ID(), blockID, actualContains, expectedContains)
			}

			return nil
		})
	}
}

// AssertStorageCommitmentTransactions asserts that the given transactionIDs were included in the commitment identified by the given slot.
func (t *TestSuite) AssertStorageCommitmentTransactions(slot iotago.SlotIndex, expectedTransactionIDs iotago.TransactionIDs, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error loading commitment for slot: %d", node.Name, slot)
			}

			committedSlot, err := node.Protocol.Engines.Main.Get().CommitmentAPI(storedCommitment.ID())
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error getting committed slot for commitment: %s", node.Name, storedCommitment.ID())
			}

			acceptedTransactions, err := committedSlot.TransactionIDs()
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error getting BlocksIDsBySlotCommitmentID for commitment: %s", node.Name, storedCommitment.ID())
			}

			if !assert.ElementsMatch(t.fakeTesting, expectedTransactionIDs, acceptedTransactions) {
				return ierrors.Errorf("AssertStorageCommitmentTransactionAccepted: %s: commitment %s's actual and expected transaction IDs do not match; actual: %s, expected: %s", node.Name, storedCommitment.ID(), acceptedTransactions, expectedTransactionIDs)
			}

			return nil
		})
	}
}

// AssertStorageCommitmentTransactionAccepted asserts that the given transaction ID has the expected containment status in the commitment identified by the given slot.
func (t *TestSuite) AssertStorageCommitmentTransactionAccepted(slot iotago.SlotIndex, transactionID iotago.TransactionID, expectedContains bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			storedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error loading commitment for slot: %d", node.Name, slot)
			}

			committedSlot, err := node.Protocol.Engines.Main.Get().CommitmentAPI(storedCommitment.ID())
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error getting committed slot for commitment: %s", node.Name, storedCommitment.ID())
			}

			acceptedTransactions, err := committedSlot.TransactionIDs()
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: error getting BlocksIDsBySlotCommitmentID for commitment: %s", node.Name, storedCommitment.ID())
			}

			actualContains := contains(acceptedTransactions, transactionID)
			if actualContains != expectedContains {
				return ierrors.Wrapf(err, "AssertStorageCommitmentTransactionAccepted: %s: commitment %s's actual and expected containment of transaction id %s do not match; actual contained status: %t, expected: %t", node.Name, storedCommitment.ID(), transactionID, actualContains, expectedContains)
			}

			return nil
		})
	}
}

func contains[T comparable](collection []T, elem T) bool {
	for _, item := range collection {
		if item == elem {
			return true
		}
	}

	return false
}
