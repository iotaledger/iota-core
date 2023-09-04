package protocol

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
	chainManager := NewChains(rootCommitment)

	chainManager.ChainSwitching.heaviestAttestedCandidate.OnUpdate(func(oldValue, newValue *Chain) {
		fmt.Println("CandidateChain", oldValue, newValue)
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
	require.True(t, commitment1Metadata.Solid().WasTriggered())

	commitment3Metadata := chainManager.ProcessCommitment(commitment3)
	require.True(t, commitment1Metadata.Solid().WasTriggered())
	require.False(t, commitment3Metadata.Solid().WasTriggered())

	commitment2Metadata := chainManager.ProcessCommitment(commitment2)
	require.True(t, commitment1Metadata.Solid().WasTriggered())
	require.True(t, commitment2Metadata.Solid().WasTriggered())
	require.True(t, commitment3Metadata.Solid().WasTriggered())

	commitment2Metadata.Verified().Trigger()
	require.True(t, commitment3Metadata.InSyncRange().Get())
	require.False(t, commitment3Metadata.RequestBlocks().Get())
	require.True(t, commitment3Metadata.Solid().WasTriggered())
	require.Equal(t, iotago.SlotIndex(3), commitment3Metadata.Chain().LatestCommitment().Index())
	require.Equal(t, uint64(3), commitment3Metadata.Chain().ClaimedWeight().Get())

	commitment3aMetadata := chainManager.ProcessCommitment(commitment3a)

	fmt.Println(commitment3aMetadata.RequestAttestations().Get())
	fmt.Println("TRIGGER ATTESTATION")
	commitment3Metadata.Parent().Get().Attested().Trigger()
	fmt.Println(commitment3aMetadata.RequestAttestations().Get())

	fmt.Println(commitment3aMetadata.Attested().Trigger())
	fmt.Println(commitment3aMetadata.RequestAttestations().Get())
}
