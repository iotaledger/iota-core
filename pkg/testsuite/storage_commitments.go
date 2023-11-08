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
				storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitment.Slot)
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
			storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(index)
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

func (t *TestSuite) AssertStorageCommitmentBlocks(slot iotago.SlotIndex, expectedBlocksBySlotCommitmentID map[iotago.CommitmentID]iotago.BlockIDs, nodes ...*mock.Node) {
	mustNodes(nodes)

	t.Eventually(func() error {
		for _, node := range nodes {
			storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error loading commitment for slot: %d", node.Name, slot)
			}

			committedSlot, err := node.Protocol.MainEngineInstance().CommittedSlot(storedCommitment.ID())
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error getting committed slot for commitment: %s", node.Name, storedCommitment.ID())
			}

			committedBlocksBySlotCommitmentID, err := committedSlot.BlocksIDsBySlotCommitmentID()
			if err != nil {
				return ierrors.Wrapf(err, "AssertStorageCommitmentBlocks: %s: error getting committed blocks for slot: %d", node.Name, slot)
			}

			if len(committedBlocksBySlotCommitmentID) == 0 {
				committedBlocksBySlotCommitmentID = nil
			}

			if len(expectedBlocksBySlotCommitmentID) == 0 {
				expectedBlocksBySlotCommitmentID = nil
			}

			if !assert.Equal(t.fakeTesting, committedBlocksBySlotCommitmentID, expectedBlocksBySlotCommitmentID) {
				return ierrors.Errorf("AssertStorageCommitmentBlocks: %s: expected %s, got %s", node.Name, expectedBlocksBySlotCommitmentID, committedBlocksBySlotCommitmentID)
			}
		}

		return nil
	})
}
