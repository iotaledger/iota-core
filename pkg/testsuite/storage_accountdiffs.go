package testsuite

import (
	"github.com/google/go-cmp/cmp"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertStorageAccountDiffs(slotIndex iotago.SlotIndex, accountDiffs map[iotago.AccountID]*model.AccountDiff, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		for accountID, diffChange := range accountDiffs {
			t.Eventually(func() error {
				store, err := node.Protocol.MainEngineInstance().Storage.AccountDiffs(slotIndex)
				if err != nil {
					return ierrors.Wrapf(err, "AssertStorageAccountDiffs: %s: failed to load accounts diff for slot %d", node.Name, slotIndex)
				}

				storedDiffChange, _, err := store.Load(accountID)
				if err != nil {
					return ierrors.Wrapf(err, "AssertStorageAccountDiffs: %s: error loading account diff: %s", node.Name, accountID)
				}
				// todo finish this, connect to other tests, is cmp enough
				if !cmp.Equal(diffChange, storedDiffChange) {
					return ierrors.Errorf("AssertStorageAccountDiffs: %s: expected %v, got %v", node.Name, diffChange, storedDiffChange)
				}

				return nil
			})
		}
	}
}
