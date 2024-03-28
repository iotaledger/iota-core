package testsuite

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertSnapshotImported(imported bool, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if imported != node.Protocol.Engines.Main.Get().Storage.Settings().IsSnapshotImported() {
				return ierrors.Errorf("AssertSnapshotImported: %s: expected %v, got %v", node.Name, imported, node.Protocol.Engines.Main.Get().Storage.Settings().IsSnapshotImported())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertProtocolParameters(parameters iotago.ProtocolParameters, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if !parameters.Equals(node.Protocol.CommittedAPI().ProtocolParameters()) {
				return ierrors.Errorf("AssertProtocolParameters: %s: expected %s, got %s", node.Name, parameters, node.Protocol.CommittedAPI().ProtocolParameters())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitment(commitment *iotago.Commitment, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if !commitment.Equals(node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment()) {
				return ierrors.Errorf("AssertLatestCommitment: %s: expected %s, got %s", node.Name, commitment, node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Commitment())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertCommitmentSlotIndexExists(slot iotago.SlotIndex, clients ...mock.Client) {
	for _, client := range clients {
		t.Eventually(func() error {
			latestCommitment, err := client.CommitmentByID(context.Background(), iotago.EmptyCommitmentID)
			if err != nil {
				return ierrors.Errorf("AssertCommitmentSlotIndexExists: error loading latest commitment: %w", err)
			}

			if latestCommitment.Slot < slot {
				return ierrors.Errorf("AssertCommitmentSlotIndexExists: commitment with at least %v not found in settings.LatestCommitment()", slot)
			}

			cm, err := client.CommitmentBySlot(context.Background(), slot)
			if err != nil {
				return ierrors.Errorf("AssertCommitmentSlotIndexExists: expected %v, got error %v", slot, err)
			}

			if cm == nil {
				return ierrors.Errorf("AssertCommitmentSlotIndexExists: commitment at index %v not found", slot)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitmentSlotIndex(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			latestCommittedSlotSettings := node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().Slot()
			if slot != latestCommittedSlotSettings {
				return ierrors.Errorf("AssertLatestCommitmentSlotIndex: %s: expected %v, got %v in settings", node.Name, slot, latestCommittedSlotSettings)
			}
			latestCommittedSlotSyncManager := node.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Slot()
			if slot != latestCommittedSlotSyncManager {
				return ierrors.Errorf("AssertLatestCommitmentSlotIndex: %s: expected %v, got %v in sync manager", node.Name, slot, latestCommittedSlotSyncManager)
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestCommitmentCumulativeWeight(cw uint64, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if cw != node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().CumulativeWeight() {
				return ierrors.Errorf("AssertLatestCommitmentCumulativeWeight: %s: expected %v, got %v", node.Name, cw, node.Protocol.Engines.Main.Get().Storage.Settings().LatestCommitment().CumulativeWeight())
			}

			return nil
		})
	}
}

func (t *TestSuite) AssertLatestFinalizedSlot(slot iotago.SlotIndex, nodes ...*mock.Node) {
	mustNodes(nodes)

	for _, node := range nodes {
		t.Eventually(func() error {
			if slot != node.Protocol.Engines.Main.Get().Storage.Settings().LatestFinalizedSlot() {
				return ierrors.Errorf("AssertLatestFinalizedSlot: %s: expected %d, got %d from settings", node.Name, slot, node.Protocol.Engines.Main.Get().Storage.Settings().LatestFinalizedSlot())
			}

			if slot != node.Protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot() {
				return ierrors.Errorf("AssertLatestFinalizedSlot: %s: expected %d, got %d from SyncManager", node.Name, slot, node.Protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot())
			}

			return nil
		})
	}
}
