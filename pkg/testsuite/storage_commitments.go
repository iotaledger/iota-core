package testsuite

import (
	"github.com/google/go-cmp/cmp"

	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageCommitments(commitments []*iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, commitment := range commitments {
			var storedCommitment *model.Commitment
			var err error
			t.Eventuallyf(func() bool {
				storedCommitment, err = node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitment.Index)
				return err == nil
			}, "AssertStorageCommitments: %s: error loading commitment: %s", node.Name, commitment.MustID(), err)

			t.Eventuallyf(func() bool {
				storedCommitment, err = node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitment.Index)
				return cmp.Equal(*commitment, *storedCommitment.Commitment())
			}, "AssertStorageCommitments: %s: expected %s, got %s", node.Name, commitment, storedCommitment)
		}
	}
}
