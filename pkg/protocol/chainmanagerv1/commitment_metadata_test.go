package chainmanagerv1

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestCommitment(t *testing.T) {
	testAPI := tpkg.TestAPI

	rootCommitment := model.NewEmptyCommitment(testAPI)
	rootCommitmentMetadata := NewCommitmentMetadata(rootCommitment)
	rootCommitmentMetadata.Solid().Trigger()
	rootCommitmentMetadata.Verified().Trigger()
	rootCommitmentMetadata.BelowSyncThreshold().Trigger()
	rootCommitmentMetadata.BelowWarpSyncThreshold().Trigger()
	rootCommitmentMetadata.BelowLatestVerifiedCommitment().Trigger()

	rootChain := NewChain(rootCommitmentMetadata)
	rootCommitmentMetadata.Chain().Set(rootChain)

	commitment1, err := model.CommitmentFromCommitment(iotago.NewCommitment(1, rootCommitment.Index()+1, rootCommitment.ID(), rootCommitmentMetadata.RootsID(), 1, 1), testAPI)
	require.NoError(t, err)

	commitment1Metadata := NewCommitmentMetadata(commitment1)
	commitment1Metadata.RegisterParent(rootCommitmentMetadata)

	require.True(t, commitment1Metadata.Solid().Get())
	require.Equal(t, rootChain, commitment1Metadata.Chain().Get())
	require.True(t, commitment1Metadata.AboveLatestVerifiedCommitment().Get())

	commitment2, err := model.CommitmentFromCommitment(iotago.NewCommitment(1, commitment1Metadata.Index()+1, rootCommitment.ID(), rootCommitmentMetadata.RootsID(), 1, 1), testAPI)
	require.NoError(t, err)

	commitment2Metadata := NewCommitmentMetadata(commitment2)
	commitment2Metadata.RegisterParent(commitment1Metadata)

	require.True(t, commitment2Metadata.Solid().Get())
	require.Equal(t, rootChain, commitment2Metadata.Chain().Get())
	require.True(t, commitment2Metadata.AboveLatestVerifiedCommitment().Get())

	commitment1Metadata.Verified().Trigger()

	require.False(t, commitment1Metadata.AboveLatestVerifiedCommitment().Get())
	require.True(t, commitment2Metadata.Solid().Get())
	require.Equal(t, rootChain, commitment2Metadata.Chain().Get())
	require.True(t, commitment2Metadata.AboveLatestVerifiedCommitment().Get())
}
