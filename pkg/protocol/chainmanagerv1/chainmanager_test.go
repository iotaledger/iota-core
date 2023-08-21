package chainmanagerv1

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestChainManager(t *testing.T) {
	testAPI := tpkg.TestAPI

	chainManager := NewChainManager()

	rootCommitment := lo.PanicOnErr(chainManager.SetRootCommitment(model.NewEmptyCommitment(testAPI)))
	commitment1Metadata := chainManager.ProcessCommitment(lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		rootCommitment.Index()+1,
		rootCommitment.ID(),
		rootCommitment.RootsID(),
		1,
		1,
	), testAPI)))

	commitment2 := lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		commitment1Metadata.Index()+1,
		commitment1Metadata.ID(),
		commitment1Metadata.RootsID(),
		2,
		2,
	), testAPI))

	commitment3Metadata := chainManager.ProcessCommitment(lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		commitment2.Index()+1,
		commitment2.ID(),
		commitment2.RootsID(),
		2,
		2,
	), testAPI)))

	require.True(t, commitment1Metadata.Solid().Get())
	require.False(t, commitment3Metadata.Solid().Get())

	commitment2Metadata := chainManager.ProcessCommitment(commitment2)

	require.True(t, commitment1Metadata.Solid().Get())
	require.True(t, commitment2Metadata.Solid().Get())
	require.True(t, commitment3Metadata.Solid().Get())

	commitment2Metadata.Verified().Trigger()

	require.True(t, commitment3Metadata.AboveLatestVerifiedCommitment().Get())
	require.True(t, commitment3Metadata.BelowSyncThreshold().Get())

	require.Equal(t, iotago.SlotIndex(3), commitment3Metadata.Chain().Get().latestCommitmentIndex.Get())
}
