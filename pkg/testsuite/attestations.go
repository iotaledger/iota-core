package testsuite

import (
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertAttestationsForSlot(slotIndex iotago.SlotIndex, blocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedAttestations := make([]iotago.BlockID, len(blocks))
	for i, block := range blocks {
		att := iotago.NewAttestation(block.Block())
		blockID, err := att.BlockID(t.ProtocolParameters.SlotTimeProvider())
		require.NoError(t.Testing, err)
		expectedAttestations[i] = blockID
	}

	for _, node := range nodes {
		t.Eventually(func() error {
			attestationTree, err := node.Protocol.MainEngineInstance().Attestations.Get(slotIndex)
			if err != nil {
				return errors.Wrapf(err, "AssertAttestationsForSlot: %s: error loading attestation tree for slot %d", node.Name, slotIndex)
			}

			storedAttestations := make([]iotago.BlockID, 0)
			err = attestationTree.Stream(func(key iotago.AccountID, att *iotago.Attestation) bool {
				blockID, err := att.BlockID(t.ProtocolParameters.SlotTimeProvider())
				require.NoError(t.Testing, err)
				storedAttestations = append(storedAttestations, blockID)

				return true
			})
			if err != nil {
				return errors.Wrapf(err, "AssertAttestationsForSlot: %s: error iterating over attestation tree", node.Name)
			}

			if len(expectedAttestations) != len(storedAttestations) {
				return errors.Errorf("AssertAttestationsForSlot: %s: expected %d attestation(s), got %d", node.Name, len(expectedAttestations), len(storedAttestations))
			}

			if !assert.ElementsMatch(t.fakeTesting, expectedAttestations, storedAttestations) {
				return errors.Errorf("AssertAttestationsForSlot: %s: expected attestation(s) %s, got %s", node.Name, expectedAttestations, storedAttestations)
			}

			return nil
		})
	}
}
