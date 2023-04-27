package testsuite

import (
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(t.Testing, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported(), "AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported())
	}
}

func (t *TestSuite) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(t.Testing, parameters, *node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters(), "AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters.String(), node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().String())
	}
}

func (t *TestSuite) AssertLatestCommitment(commitment *iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Truef(t.Testing, commitment.Equals(node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()), "AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment.String(), node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().String())
	}
}

func (t *TestSuite) AssertLatestCommitmentSlotIndex(slot int, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.EqualValuesf(t.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index, "AssertLatestCommitmentSlotIndex: %s: expected %s, got %s", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index)
	}
}

func (t *TestSuite) AssertLatestStateMutationSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(t.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot(), "AssertLatestStateMutationSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot())
	}
}

func (t *TestSuite) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(t.Testing, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot(), "AssertLatestFinalizedSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot())
	}
}

func (t *TestSuite) AssertChainID(chainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		require.Equalf(t.Testing, chainID, node.Protocol.MainEngineInstance().Storage.Settings().ChainID(), "AssertChainID: %s: expected %s, got %s", node.Name, chainID.String(), node.Protocol.MainEngineInstance().Storage.Settings().ChainID().String())
	}
}
