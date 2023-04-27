package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (f *Framework) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported(), "AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported())
	}
}

func (f *Framework) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, parameters, *node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(), "AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters.String(), node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().String())
	}
}

func (f *Framework) AssertLatestCommitment(commitment *iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Truef(f.Testing, commitment.Equals(node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()), "AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment.String(), node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().String())
	}
}

func (f *Framework) AssertLatestCommitmentSlotIndex(slot int, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.EqualValuesf(f.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index, "AssertLatestCommitmentSlotIndex: %s: expected %s, got %s", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)
	}
}

func (f *Framework) AssertLatestStateMutationSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot(), "AssertLatestStateMutationSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot())
	}
}

func (f *Framework) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(), "AssertLatestFinalizedSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot())
	}
}

func (f *Framework) AssertChainID(chainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(f.Testing, chainID, node.Protocol.MainEngineInstance().Storage.Settings().ChainID(), "AssertChainID: %s: expected %s, got %s", node.Name, chainID.String(), node.Protocol.MainEngineInstance().Storage.Settings().ChainID().String())
	}
}
