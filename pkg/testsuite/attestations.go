package testsuite

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertAttestationsForSlot(slotIndex iotago.SlotIndex, blocks []*blocks.Block, nodes ...*mock.Node) {
	mustNodes(nodes)

	expectedAttestations := make([]iotago.BlockID, len(blocks))
	for i, block := range blocks {
		att := iotago.NewAttestation(t.API, block.ProtocolBlock())
		blockID, err := att.BlockID()
		require.NoError(t.Testing, err)
		expectedAttestations[i] = blockID
	}

	for _, node := range nodes {
		t.Eventually(func() error {
			attestationTree, err := node.Protocol.MainEngineInstance().Attestations.GetMap(slotIndex)
			if err != nil {
				return ierrors.Wrapf(err, "AssertAttestationsForSlot: %s: error loading attestation tree for slot %d", node.Name, slotIndex)
			}

			storedAttestations := make([]iotago.BlockID, 0)
			err = attestationTree.Stream(func(key iotago.AccountID, att *iotago.Attestation) error {
				blockID, err := att.BlockID()
				require.NoError(t.Testing, err)
				storedAttestations = append(storedAttestations, blockID)

				return nil
			})
			if err != nil {
				return ierrors.Wrapf(err, "AssertAttestationsForSlot: %s: error iterating over attestation tree", node.Name)
			}

			if len(expectedAttestations) != len(storedAttestations) {
				return ierrors.Errorf("AssertAttestationsForSlot: %s: expected %d attestation(s), got %d", node.Name, len(expectedAttestations), len(storedAttestations))
			}

			if !assert.ElementsMatch(t.fakeTesting, expectedAttestations, storedAttestations) {
				return ierrors.Errorf("AssertAttestationsForSlot: %s: expected attestation(s) %s, got %s", node.Name, expectedAttestations, storedAttestations)
			}

			return nil
		})
	}
}
