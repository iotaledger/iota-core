package testsuite

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *TestSuite) AssertAttestationsForSlot(slot iotago.SlotIndex, blocks []*blocks.Block, nodes ...*mock.Node) {
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
			attestationTree, err := node.Protocol.Engines.Main.Get().Attestations.GetMap(slot)
			if err != nil {
				return ierrors.Wrapf(err, "AssertAttestationsForSlot: %s: error loading attestation tree for slot %d", node.Name, slot)
			}

			storedAttestations := make([]iotago.BlockID, 0)
			//nolint:revive
			err = attestationTree.Stream(func(key iotago.AccountID, att *iotago.Attestation) error {
				blockID, err := att.BlockID()
				if err != nil {
					return ierrors.Wrapf(err, "failed to stream attestationTree: %s, slot: %d", node.Name, slot)
				}
				storedAttestations = append(storedAttestations, blockID)

				return nil
			})
			if err != nil {
				return ierrors.Wrapf(err, "AssertAttestationsForSlot: %s: %s error iterating over attestation tree", node.Name, slot)
			}

			if !assert.ElementsMatch(t.fakeTesting, expectedAttestations, storedAttestations) {
				return ierrors.Errorf("AssertAttestationsForSlot: %s: %s expected attestation(s) %s, got %s", node.Name, slot, expectedAttestations, storedAttestations)
			}

			if len(expectedAttestations) != len(storedAttestations) {
				return ierrors.Errorf("AssertAttestationsForSlot: %s: %s expected %d attestation(s), got %d", node.Name, slot, len(expectedAttestations), len(storedAttestations))
			}

			return nil
		})
	}
}
