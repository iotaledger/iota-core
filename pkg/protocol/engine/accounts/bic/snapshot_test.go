package bic

import (
	"bytes"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSlotDiffSnapshotBytes(t *testing.T) {
	accountID := utils.RandAccountID()
	bicDiffChange := prunable.BicDiffChange{
		Change:              10,
		PreviousUpdatedTime: utils.RandSlotIndex(),
		PubKeysAdded:        utils.RandPubKeys(),
		PubKeysRemoved:      utils.RandPubKeys(),
	}
	destroyed := false
	snapshotBytes := slotDiffSnapshotBytes(accountID, bicDiffChange, destroyed)
	buf := bytes.NewReader(snapshotBytes)
	bicDiffChangeRead, accountIDRead, destroyedRead, err := slotDiffSnapshotReader(buf)
	require.NoError(t, err)
	require.Equal(t, bicDiffChange, bicDiffChangeRead)
	require.Equal(t, accountID, accountIDRead)
	require.Equal(t, destroyed, destroyedRead)

}

func TestBICSnapshotBytes(t *testing.T) {

}

func TestTargetBICCreation(t *testing.T) {

}
