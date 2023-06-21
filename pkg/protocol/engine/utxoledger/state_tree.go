package utxoledger

import (
	"bytes"

	"github.com/iotaledger/hive.go/ads"
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

func (s *stateTreeMetadata) FromBytes(b []byte) (int, error) {
	var err error
	s.Time, err = iotago.SlotIndexFromBytes(b)
	if err != nil {
		return 0, err
	}

	return 8, nil
}

func (s stateTreeMetadata) Bytes() ([]byte, error) {
	ms := marshalutil.New(8)
	ms.WriteBytes(s.Time.Bytes())

	return ms.Bytes(), nil
}

func (m *Manager) StateTreeRoot() iotago.Identifier {
	return iotago.Identifier(m.stateTree.Root())
}

func (m *Manager) CheckStateTree() bool {
	comparisonTree := ads.NewMap[iotago.OutputID, stateTreeMetadata](mapdb.NewMapDB())

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
