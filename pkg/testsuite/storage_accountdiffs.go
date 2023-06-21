package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageAccountDiffs(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*prunable.AccountDiff, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for accountID, diffChange := range accountDiffs {
			t.Eventually(func() error {
				storedDiffChange, _, err := node.Protocol.MainEngineInstance().Storage.AccountDiffs(slotIndex).Load(accountID)
				if err != nil {
					return errors.Wrapf(err, "AssertStorageAccountDiffs: %s: error loading account diff: %s", node.Name, accountID.String())
				}
				// todo finish this, connect to other tests, is cmp enough
				if !cmp.Equal(diffChange, storedDiffChange) {
					return errors.Errorf("AssertStorageAccountDiffs: %s: expected %v, got %v", node.Name, diffChange, storedDiffChange)
				}

				return nil
			})
		}
	}
}
