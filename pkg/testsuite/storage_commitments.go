package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageCommitments(commitments []*iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, commitment := range commitments {
			t.Eventually(func() error {
				storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitment.Index)
				if err != nil {
					return errors.Wrapf(err, "AssertStorageCommitments: %s: error loading commitment: %s", node.Name, commitment.MustID())
				}

				if !cmp.Equal(*commitment, *storedCommitment.Commitment()) {
					return errors.Errorf("AssertStorageCommitments: %s: expected %s, got %s", node.Name, commitment, storedCommitment)
				}

				return nil
			})
		}
	}
}

func (t *TestSuite) AssertStorageCommitmentAtIndex(index iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	t.Eventually(func() error {
		var commitment *iotago.Commitment
		for _, node := range nodes {
			storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(index)
			if err != nil {
				return errors.Wrapf(err, "AssertStorageCommitmentAtIndex: %s: error loading commitment: %s", node.Name, commitment.MustID())
			}

			if commitment == nil {
				commitment = storedCommitment.Commitment()
				continue
			}

			if !cmp.Equal(*commitment, *storedCommitment.Commitment()) {
				return errors.Errorf("AssertStorageCommitmentAtIndex: %s: expected %s, got %s", node.Name, commitment, storedCommitment)
			}
		}

		return nil
	})
}
