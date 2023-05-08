package ledgerstate

import (
	"bytes"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

type stateTreeMetadata struct {
	Time time.Time
}

func newStateMetadata(output *Output) *stateTreeMetadata {
	return &stateTreeMetadata{
		Time: output.TimestampCreated(),
	}
}

func (s *stateTreeMetadata) FromBytes(b []byte) (int, error) {
	ms := marshalutil.New(b)
	ts, err := ms.ReadInt64()
	if err != nil {
		return 0, err
	}

	s.Time = time.Unix(0, ts)

	return 8, nil
}

func (s stateTreeMetadata) Bytes() ([]byte, error) {
	ms := marshalutil.New(8)
	ms.WriteInt64(s.Time.UnixNano())
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
