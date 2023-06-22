package rewards

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/utils"
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
	positionedWriter := utils.NewPositionedWriter(writer)
	// TODO export performance factor up to the epoch start

	err := m.exportPoolRewards(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool rewards")
	}

	err = m.exportPoolsStats(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool stats")
	}
	return nil
}

func (m *Manager) exportPoolRewards(pWriter *utils.PositionedWriter, epoch iotago.EpochIndex) error {
	// export all stored pools
	// intheory we could save the epoch count only once, because stats and rewards should be the same length
	var epochCount uint64
	if err := pWriter.WriteValue("accounts count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	var innerErr error
	// TODO can I iterete over the undeneath db directly?
	err := m.rewardBaseStore.Iterate([]byte{}, func(key []byte, value []byte) bool {
		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
		if epochIndex > epoch {
			// continue
			return true
		}
		if err := pWriter.WriteBytes(key); err != nil {
			innerErr = errors.Wrap(err, "unable to write epoch index")
			return false
		}
		if err := pWriter.WriteBytes(value); err != nil {
			innerErr = errors.Wrap(err, "unable to write epoch index")
			return false
		}
		epochCount++
		return true
	})
	if err != nil {
		return errors.Wrapf(err, "unable to iterate over reward base store: %s", innerErr)
	}
	if err := pWriter.WriteValueAtBookmark("accounts count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	return nil

}

func (m *Manager) exportPoolsStats(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("accounts count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	// export all stored pools
	var innerErr error
	if err := m.poolStatsStore.Iterate([]byte{}, func(key []byte, value []byte) bool {
		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
		if epochIndex > targetEpoch {
			// continue
			return true
		}
		if err := pWriter.WriteBytes(key); err != nil {
			innerErr = errors.Wrap(err, "unable to write epoch index")
			return false
		}
		if err := pWriter.WriteBytes(value); err != nil {
			err = errors.Wrap(err, "unable to write slot diffs count")
			return false
		}
		epochCount++
		return true
	}); err != nil {
		return errors.Wrapf(err, "unable to iterate over slot diffs: %v", innerErr)
	}
	if err := pWriter.WriteValueAtBookmark("accounts count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	return nil
}
