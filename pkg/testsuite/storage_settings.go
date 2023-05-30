package testsuite

import (
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if imported != node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported() {
				return errors.Errorf("AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.MainEngineInstance().Storage.Settings().SnapshotImported())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if !cmp.Equal(parameters, *node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters()) {
				return errors.Errorf("AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters.String(), node.Protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().String())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitment(commitment *iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if !commitment.Equals(node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()) {
				return errors.Errorf("AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment.String(), node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().String())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitmentSlotIndex(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if slot != node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index() {
				return errors.Errorf("AssertLatestCommitmentSlotIndex: %s: expected %v, got %v", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitmentCumulativeWeight(cw uint64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if cw != node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().CumulativeWeight() {
				return errors.Errorf("AssertLatestCommitmentCumulativeWeight: %s: expected %v, got %v", node.Name, cw, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().CumulativeWeight())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestStateMutationSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if slot != node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot() {
				return errors.Errorf("AssertLatestStateMutationSlot: %s: expected %d, got %d", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestStateMutationSlot())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if slot != node.Protocol.MainEngineInstance().SlotGadget.LatestFinalizedSlot() {
				return errors.Errorf("AssertLatestFinalizedSlot: %s: expected %d, got %d from slot gadget", node.Name, slot, node.Protocol.MainEngineInstance().SlotGadget.LatestFinalizedSlot())
			}

			if slot != node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot() {
				return errors.Errorf("AssertLatestFinalizedSlot: %s: expected %d, got %d from settings", node.Name, slot, node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertChainID(expectedChainID iotago.CommitmentID, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			actualChainID := node.Protocol.MainEngineInstance().ChainID()
			if expectedChainID != node.Protocol.MainEngineInstance().ChainID() {
				return errors.Errorf("AssertChainID: %s: expected %s (index: %d), got %s (index: %d)", node.Name, expectedChainID.String(), expectedChainID.Index(), actualChainID, actualChainID.Index())
			}

			return nil
		})
	}
}
