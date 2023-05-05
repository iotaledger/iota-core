package testsuite

import (
	"github.com/google/go-cmp/cmp"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return imported == node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported()
		}, "AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported())
	}
}

func (t *TestSuite) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return cmp.Equal(parameters, *node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters())
		}, "AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters.String(), node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().String())
	}
}

func (t *TestSuite) AssertLatestCommitment(commitment *iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return commitment.Equals(node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment())
		}, "AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment.String(), node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().String())
	}
}

func (t *TestSuite) AssertLatestCommitmentSlotIndex(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return cmp.Equal(slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index())
		}, "AssertLatestCommitmentSlotIndex: %s: expected %v, got %v", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index())
	}
}

func (t *TestSuite) AssertLatestStateMutationSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return cmp.Equal(slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot())
		}, "AssertLatestStateMutationSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot())
	}
}

func (t *TestSuite) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return cmp.Equal(slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot())
		}, "AssertLatestFinalizedSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot())
	}
}

func (t *TestSuite) AssertChainID(chainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventuallyf(func() bool {
			return chainID == node.Protocol.MainEngineInstance().Storage.Settings().ChainID()
		}, "AssertChainID: %s: expected %s, got %s", node.Name, chainID.String(), node.Protocol.MainEngineInstance().Storage.Settings().ChainID().String())
	}
}
