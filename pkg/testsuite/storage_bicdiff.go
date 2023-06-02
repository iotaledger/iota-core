package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageBicDiffs(slotIndex iotago.SlotIndex, bicDiffs map[iotago.AccountID]*prunable.BicDiffChange, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for accountID, diffChange := range bicDiffs {
			t.Eventually(func() error {
				storedDiffChange, _, err := node.Protocol.MainEngineInstance().Storage.BicDiffs(slotIndex).Load(accountID)
				if err != nil {
					return errors.Wrapf(err, "AssertStorageBicDiffs: %s: error loading bic diff: %s", node.Name, accountID.String())
				}
				// todo finish this, connect to other tests, is cmp enough
				if !cmp.Equal(diffChange, storedDiffChange) {
					return errors.Errorf("AssertStorageBicDiffs: %s: expected %v, got %v", node.Name, diffChange, storedDiffChange)
				}
				return nil
			})

		}
	}
}
