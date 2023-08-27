package chainmanagerv1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/tpkg"
)

func TestChainManager(t *testing.T) {
	testAPI := tpkg.TestAPI
	rootCommitment := model.NewEmptyCommitment(testAPI)
	chainManager := NewChainManager(rootCommitment)

	chainManager.HeaviestCandidateChain().OnUpdate(func(oldValue, newValue *Chain) {
		fmt.Println("HeaviestCandidateChain", oldValue, newValue)
	})

	chainManager.HeaviestAttestedCandidateChain().OnUpdate(func(oldValue, newValue *Chain) {
		fmt.Println("HeaviestAttestedCandidateChain", oldValue, newValue)
	})

	chainManager.HeaviestVerifiedCandidateChain().OnUpdate(func(oldValue, newValue *Chain) {
		fmt.Println("HeaviestVerifiedCandidateChain", oldValue, newValue)
	})

	commitment1 := lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		rootCommitment.Index()+1,
		rootCommitment.ID(),
		rootCommitment.RootsID(),
		1,
		1,
	), testAPI))

	commitment2 := lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		commitment1.Index()+1,
		commitment1.ID(),
		commitment1.RootsID(),
		2,
		2,
	), testAPI))

	commitment3 := lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		commitment2.Index()+1,
		commitment2.ID(),
		commitment2.RootsID(),
		3,
		2,
	), testAPI))

	commitment3a := lo.PanicOnErr(model.CommitmentFromCommitment(iotago.NewCommitment(testAPI.Version(),
		commitment2.Index()+1,
		commitment2.ID(),
		commitment2.RootsID(),
		40,
		2,
	), testAPI))

	commitment1Metadata := chainManager.ProcessCommitment(commitment1)
	require.True(t, commitment1Metadata.Solid().Get())

	commitment3Metadata := chainManager.ProcessCommitment(commitment3)
	require.True(t, commitment1Metadata.Solid().Get())
	require.False(t, commitment3Metadata.Solid().Get())

	commitment2Metadata := chainManager.ProcessCommitment(commitment2)
	require.True(t, commitment1Metadata.Solid().Get())
	require.True(t, commitment2Metadata.Solid().Get())
	require.True(t, commitment3Metadata.Solid().Get())

	commitment2Metadata.Verified().Trigger()
	require.True(t, commitment3Metadata.AboveLatestVerifiedCommitment().Get())
	require.True(t, commitment3Metadata.BelowSyncThreshold().Get())
	require.Equal(t, iotago.SlotIndex(3), commitment3Metadata.Chain().Get().Commitments().Latest().Index())
	require.Equal(t, uint64(3), commitment3Metadata.Chain().Get().Weight().Claimed())

	commitment3aMetadata := chainManager.ProcessCommitment(commitment3a)
	commitment3aMetadata.Attested().Trigger()
	commitment3aMetadata.Verified().Trigger()
	fmt.Println(commitment3aMetadata.Chain().Get())
}
