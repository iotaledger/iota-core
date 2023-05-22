package testsuite

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertAttestationsForSlot(slotIndex iotago.SlotIndex, blocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedAttestations := make([]*iotago.Attestation, len(blocks))
	for i, block := range blocks {
		expectedAttestations[i] = iotago.NewAttestation(block.Block())
	}

	for _, node := range nodes {
		t.Eventually(func() error {
			attestationTree, err := node.Protocol.MainEngineInstance().Attestation.Get(slotIndex)
			if err != nil {
				return errors.Wrapf(err, "AssertStorageAttestationsForSlot: %s: error loading attestation tree for slot %d", node.Name, slotIndex)
			}

			storedAttestations := make([]*iotago.Attestation, 0)
			err = attestationTree.Stream(func(key iotago.AccountID, value *iotago.Attestation) bool {
				storedAttestations = append(storedAttestations, value)
				return true
			})
			if err != nil {
				return errors.Wrapf(err, "AssertStorageAttestationsForSlot: %s: error iterating over attestation tree", node.Name)
			}

			if len(expectedAttestations) != len(storedAttestations) {
				return errors.Errorf("AssertStorageAttestationsForSlot: %s: expected %d attestation(s), got %d", node.Name, len(expectedAttestations), len(storedAttestations))
			}

			if !assert.ElementsMatch(t.fakeTesting, expectedAttestations, storedAttestations) {
				return errors.Errorf("AssertStorageAttestationsForSlot: %s: expected attestation(s) %v, got %v", node.Name, expectedAttestations, storedAttestations)
			}

			return nil
		})
	}
}
