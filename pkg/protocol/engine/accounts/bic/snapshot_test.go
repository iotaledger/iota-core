package bic

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
)

func TestSlotDiffSnapshotBytes(t *testing.T) {
	accountID := utils.RandAccountID()
	bicDiffChange := &prunable.BicDiffChange{
		Change:              10,
		PreviousUpdatedTime: utils.RandSlotIndex(),
		PubKeysAdded:        utils.RandPubKeys(),
		PubKeysRemoved:      utils.RandPubKeys(),
	}
	snapshotBytes := slotDiffSnapshotBytes(accountID, *bicDiffChange, true)
	buf := bytes.NewReader(snapshotBytes)
	bicDiffChangeRead, accountIDRead, destroyedRead, err := slotDiffSnapshotReader(buf)
	require.NoError(t, err)
	require.Equal(t, bicDiffChange, bicDiffChangeRead)
	require.Equal(t, accountID, accountIDRead)
	require.Equal(t, true, destroyedRead)
}

func TestBICSnapshotBytes(t *testing.T) {

}

func TestTargetBICCreation(t *testing.T) {

}
