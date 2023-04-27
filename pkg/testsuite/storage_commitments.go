package testsuite

import (
	"fmt"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageCommitments(commitments []*iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for _, commitment := range commitments {
			storedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitment.Index)
			if err != nil {
				panic(fmt.Sprintf("failed to load commitment %s: %s", commitment.MustID(), err.Error()))
			}
			require.Equalf(t.Testing, *commitment, *storedCommitment.Commitment(), "%s: expected %s, got %s", node.Name, commitment, storedCommitment)
		}
	}
}
