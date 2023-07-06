package testsuite

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if imported != node.Protocol.MainEngineInstance().Storage.Settings().IsSnapshotImported() {
				return errors.Errorf("AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.MainEngineInstance().Storage.Settings().IsSnapshotImported())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			//nolint:forcetypeassert // just crash if you can't do it
			if !parameters.(*iotago.V3ProtocolParameters).Equals(node.Protocol.MainEngineInstance().Storage.Settings().LatestAPI().ProtocolParameters().(*iotago.V3ProtocolParameters)) {
				return errors.Errorf("AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters, node.Protocol.MainEngineInstance().Storage.Settings().LatestAPI().ProtocolParameters())
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
				return errors.Errorf("AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment, node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment())
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

func (t *TestSuite) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
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
				return errors.Errorf("AssertChainID: %s: expected %s (index: %d), got %s (index: %d)", node.Name, expectedChainID, expectedChainID.Index(), actualChainID, actualChainID.Index())
			}

			return nil
		})
	}
}
