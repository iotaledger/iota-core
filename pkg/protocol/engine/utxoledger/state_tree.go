package utxoledger

import (
	"bytes"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

type stateTreeMetadata struct {
	Time iotago.SlotIndex
}

func newStateMetadata(output *Output) *stateTreeMetadata {
	return &stateTreeMetadata{
		Time: output.SlotCreated(),
	}
}

func stateMetadataFromBytes(b []byte) (*stateTreeMetadata, int, error) {
	s := new(stateTreeMetadata)

	var err error
	var n int
	s.Time, n, err = iotago.SlotIndexFromBytes(b)
	if err != nil {
		return nil, 0, err
	}

	return s, n, nil
}

func (s *stateTreeMetadata) Bytes() ([]byte, error) {
	ms := marshalutil.New(8)
	ms.WriteBytes(s.Time.MustBytes())

	return ms.Bytes(), nil
}

func (m *Manager) StateTreeRoot() iotago.Identifier {
	return iotago.Identifier(m.stateTree.Root())
}

func (m *Manager) CheckStateTree() bool {
	comparisonTree := ds.NewAuthenticatedMap(mapdb.NewMapDB(),
		iotago.OutputID.Bytes,
		iotago.OutputIDFromBytes,
		(*stateTreeMetadata).Bytes,
		stateMetadataFromBytes,
	)

	if err := m.ForEachUnspentOutput(func(output *Output) bool {
		comparisonTree.Set(output.OutputID(), newStateMetadata(output))

		return true
	}); err != nil {
		return false
	}

	comparisonRoot := comparisonTree.Root()
	storedRoot := m.StateTreeRoot()

	return bytes.Equal(comparisonRoot[:], storedRoot[:])
}
