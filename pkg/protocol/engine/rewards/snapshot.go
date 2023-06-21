package rewards

import (
	"encoding/binary"
	"io"

	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	// TODO: implement this

	return nil

}

func (m *Manager) Export(writer io.WriteSeeker, targetSlotIndex iotago.SlotIndex) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	targetEpoch := m.timeProvider.EpochsFromSlot(targetSlotIndex)
	// if target index is the last slot of the epoch, the epoch was committed
	if m.timeProvider.EpochEnd(targetEpoch) != targetSlotIndex {
		if targetEpoch == 1 {
			targetEpoch = 0
		} else {
			targetEpoch -= 1
		}
	}
	// TODO export performance factor up to the epoch start

	m.exportPoolRewards(writer, targetEpoch)

	m.exportPoolsStats(writer, targetEpoch)
	return nil
}

func (m *Manager) exportPoolRewards(writer io.WriteSeeker, epoch iotago.EpochIndex) {
	// export all stored pools
}

func (m *Manager) exportPoolsStats(writer io.WriteSeeker, targetEpoch iotago.EpochIndex) {
	// export all stored pools
	var innerErr error
	if err := m.poolStatsStore.Iterate([]byte{}, func(key []byte, value []byte) bool {
		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
		if epochIndex > targetEpoch {
			return true
		}
		poolsStats := new(PoolsStats)
		_, innerErr = poolsStats.FromBytes(value)
		if innerErr != nil {
			return false
		}
		return true
	}); err != nil {
		panic(err)
	}
}
