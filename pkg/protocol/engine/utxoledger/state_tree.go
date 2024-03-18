package utxoledger

import (
	"bytes"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	iotago "github.com/iotaledger/iota.go/v4"
)

type stateTreeMetadata struct {
	Slot iotago.SlotIndex
}

func newStateMetadata(output *Output) *stateTreeMetadata {
	return &stateTreeMetadata{
		Slot: output.SlotCreated(),
	}
}

func stateMetadataFromBytes(b []byte) (*stateTreeMetadata, int, error) {
	s := new(stateTreeMetadata)

	var err error
	var n int
	s.Slot, n, err = iotago.SlotIndexFromBytes(b)
	if err != nil {
		return nil, 0, err
	}

	return s, n, nil
}

func (s *stateTreeMetadata) Bytes() ([]byte, error) {
	return s.Slot.Bytes()
}

func (m *Manager) StateTreeRoot() iotago.Identifier {
	return m.stateTree.Root()
}

func (m *Manager) CheckStateTree() bool {
	comparisonTree := ads.NewMap[iotago.Identifier](mapdb.NewMapDB(),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.OutputID.Bytes,
		iotago.OutputIDFromBytes,
		(*stateTreeMetadata).Bytes,
		stateMetadataFromBytes,
	)

	if err := m.ForEachUnspentOutput(func(output *Output) bool {
		if err := comparisonTree.Set(output.OutputID(), newStateMetadata(output)); err != nil {
			panic(ierrors.Wrapf(err, "failed to set output in comparison tree, outputID: %s", output.OutputID().ToHex()))
		}

		return true
	}); err != nil {
		return false
	}

	comparisonRoot := comparisonTree.Root()
	storedRoot := m.StateTreeRoot()

	return bytes.Equal(comparisonRoot[:], storedRoot[:])
}
