package rewards

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (m *Manager) Import(reader io.ReadSeeker) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

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

	err := m.exportPerformanceFactor(positionedWriter, targetSlotIndex)
	if err != nil {
		return errors.Wrap(err, "unable to export performance factor")
	}

	err = m.exportPoolRewards(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool rewards")
	}

	err = m.exportPoolsStats(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool stats")
	}
	return nil
}

func (m *Manager) exportPerformanceFactor(pWriter *utils.PositionedWriter, targetSlot iotago.SlotIndex) error {
	m.performanceFactorsMutex.RLock()
	defer m.performanceFactorsMutex.RUnlock()

	var slotCount uint64
	if err := pWriter.WriteValue("pf slot count", slotCount, true); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	var innerErr error
	m.performanceFactorsCache.ForEach(func(slotIndex iotago.SlotIndex, pf *prunable.PerformanceFactors) bool {
		if slotIndex > targetSlot {
			// continue
			return true
		}
		var accountsCount uint64
		if err := pWriter.WriteValue("slot index", slotIndex); err != nil {
			innerErr = errors.Wrap(err, "unable to write slot index")
			return false
		}
		if err := pWriter.WriteValue("pf account count", accountsCount, true); err != nil {
			innerErr = errors.Wrap(err, "unable to write accounts count")
			return false
		}
		innerErr = pf.ForEachPerformanceFactor(func(accountID iotago.AccountID, pf uint64) error {
			if err := pWriter.WriteValue("account id", accountID); err != nil {
				return errors.Wrap(err, "unable to write account id")

			}
			if err := pWriter.WriteValue("performance factor", pf); err != nil {
				return errors.Wrap(err, "unable to write performance factor")
			}
			accountsCount++
			return nil
		})
		if innerErr != nil {
			return false
		}
		if err := pWriter.WriteValueAtBookmark("pf account count", accountsCount); err != nil {
			innerErr = errors.Wrap(err, "unable to write accounts count")
			return false
		}

		slotCount++
		return true
	})
	if innerErr != nil {
		return errors.Wrap(innerErr, "unable to write performance factors")
	}
	if err := pWriter.WriteValueAtBookmark("pf slot count", slotCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	return nil
}

func (m *Manager) exportPoolRewards(pWriter *utils.PositionedWriter, epoch iotago.EpochIndex) error {
	// export all stored pools
	// intheory we could save the epoch count only once, because stats and rewards should be the same length
	var epochCount uint64
	if err := pWriter.WriteValue("pool rewards epoch count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	var innerErr error
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
	if err = pWriter.WriteValueAtBookmark("pool rewards epoch count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	return nil

}

func (m *Manager) exportPoolsStats(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("pools stats epoch count", epochCount, true); err != nil {
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
	if err := pWriter.WriteValueAtBookmark("pools stats epoch count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write accounts count")
	}
	return nil
}
