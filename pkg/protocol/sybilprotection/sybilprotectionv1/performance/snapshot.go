package performance

import (
	"encoding/binary"
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	// TODO: should be addressed in issue #300.
	daysInYear = 365
)

func (t *Tracker) Import(reader io.ReadSeeker) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if err := t.importPerformanceFactor(reader); err != nil {
		return ierrors.Wrap(err, "unable to import performance factor")
	}

	if err := t.importPoolRewards(reader); err != nil {
		return ierrors.Wrap(err, "unable to import pool rewards")
	}

	if err := t.importPoolsStats(reader); err != nil {
		return ierrors.Wrap(err, "unable to import pool stats")
	}

	if err := t.importCommittees(reader); err != nil {
		return ierrors.Wrap(err, "unable to import committees")
	}

	return nil
}

func (t *Tracker) Export(writer io.WriteSeeker, targetSlotIndex iotago.SlotIndex) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.apiProvider.APIForSlot(targetSlotIndex).TimeProvider()
	targetEpoch := timeProvider.EpochFromSlot(targetSlotIndex)
	positionedWriter := utils.NewPositionedWriter(writer)

	// if the target index is the last slot of the epoch, the epoch was committed
	if timeProvider.EpochEnd(targetEpoch) != targetSlotIndex {
		targetEpoch--
	}

	if err := t.exportPerformanceFactor(positionedWriter, timeProvider.EpochStart(targetEpoch+1), targetSlotIndex); err != nil {
		return ierrors.Wrap(err, "unable to export performance factor")
	}

	if err := t.exportPoolRewards(positionedWriter, targetEpoch); err != nil {
		return ierrors.Wrap(err, "unable to export pool rewards")
	}

	if err := t.exportPoolsStats(positionedWriter, targetEpoch); err != nil {
		return ierrors.Wrap(err, "unable to export pool stats")
	}

	if err := t.exportCommittees(positionedWriter, targetSlotIndex); err != nil {
		return ierrors.Wrap(err, "unable to export committees")
	}

	return nil
}

func (t *Tracker) importPerformanceFactor(reader io.ReadSeeker) error {
	var slotCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &slotCount); err != nil {
		return ierrors.Wrap(err, "unable to read slot count")
	}

	for i := uint64(0); i < slotCount; i++ {
		var slotIndex iotago.SlotIndex
		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return ierrors.Wrap(err, "unable to read slot index")
		}

		var accountsCount uint64
		if err := binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return ierrors.Wrapf(err, "unable to read accounts count for slot index %d", slotIndex)
		}

		performanceFactors, err := t.validatorPerformancesFunc(slotIndex)
		if err != nil {
			return ierrors.Wrapf(err, "unable to get performance factors for slot index %d", slotIndex)
		}

		for j := uint64(0); j < accountsCount; j++ {
			var accountID iotago.AccountID
			if err = binary.Read(reader, binary.LittleEndian, &accountID); err != nil {
				return ierrors.Wrapf(err, "unable to read account id for the slot index %d", slotIndex)
			}

			var performanceFactor model.ValidatorPerformance
			if err = binary.Read(reader, binary.LittleEndian, &performanceFactor); err != nil {
				return ierrors.Wrapf(err, "unable to read performance factor for account %s and slot index %d", accountID, slotIndex)
			}

			if err = performanceFactors.Store(accountID, &performanceFactor); err != nil {
				return ierrors.Wrapf(err, "unable to store performance factor for account %s and slot index %d", accountID, slotIndex)
			}
		}
	}

	return nil
}

func (t *Tracker) importPoolRewards(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return ierrors.Wrap(err, "unable to read epoch count")
	}

	for i := uint64(0); i < epochCount; i++ {
		var epochIndex iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epochIndex); err != nil {
			return ierrors.Wrap(err, "unable to read epoch index")
		}

		rewardsTree, err := t.rewardsMap(epochIndex)
		if err != nil {
			return ierrors.Wrapf(err, "unable to get rewards tree for epoch index %d", epochIndex)
		}

		var accountsCount uint64
		if err = binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return ierrors.Wrapf(err, "unable to read accounts count for epoch index %d", epochIndex)
		}

		for j := uint64(0); j < accountsCount; j++ {
			var accountID iotago.AccountID
			if err = binary.Read(reader, binary.LittleEndian, &accountID); err != nil {
				return ierrors.Wrapf(err, "unable to read account id for the epoch index %d", epochIndex)
			}

			var reward model.PoolRewards
			if err = binary.Read(reader, binary.LittleEndian, &reward); err != nil {
				return ierrors.Wrapf(err, "unable to read reward for account %s and epoch index %d", accountID, epochIndex)
			}

			if err = rewardsTree.Set(accountID, &reward); err != nil {
				return ierrors.Wrapf(err, "unable to set reward for account %s and epoch index %d", accountID, epochIndex)
			}
		}

		if err = rewardsTree.Commit(); err != nil {
			return ierrors.Wrapf(err, "unable to commit rewards for epoch index %d", epochIndex)
		}
	}

	return nil
}

func (t *Tracker) importPoolsStats(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return ierrors.Wrap(err, "unable to read epoch count")
	}

	for i := uint64(0); i < epochCount; i++ {
		var epochIndex iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epochIndex); err != nil {
			return ierrors.Wrap(err, "unable to read epoch index")
		}

		var poolStats model.PoolsStats
		if err := binary.Read(reader, binary.LittleEndian, &poolStats); err != nil {
			return ierrors.Wrapf(err, "unable to read pool stats for epoch index %d", epochIndex)
		}

		if err := t.poolStatsStore.Store(epochIndex, &poolStats); err != nil {
			return ierrors.Wrapf(err, "unable to store pool stats for the epoch index %d", epochIndex)
		}
	}

	return nil
}

func (t *Tracker) importCommittees(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return ierrors.Wrap(err, "unable to read committees epoch count")
	}
	for i := uint64(0); i < epochCount; i++ {
		var epoch iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epoch); err != nil {
			return ierrors.Wrap(err, "unable to read epoch index")
		}

		committee, _, err := account.AccountsFromReader(reader)
		if err != nil {
			return ierrors.Wrapf(err, "unable to read committee for the epoch index %d", epoch)
		}

		if err = t.committeeStore.Store(epoch, committee); err != nil {
			return ierrors.Wrap(err, "unable to store committee")
		}
	}

	return nil
}

func (t *Tracker) exportPerformanceFactor(pWriter *utils.PositionedWriter, startSlot, targetSlot iotago.SlotIndex) error {
	t.performanceFactorsMutex.RLock()
	defer t.performanceFactorsMutex.RUnlock()

	var slotCount uint64
	if err := pWriter.WriteValue("pf slot count", slotCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write pf slot count")
	}

	for currentSlot := startSlot; currentSlot <= targetSlot; currentSlot++ {
		if err := pWriter.WriteValue("slot index", currentSlot); err != nil {
			return ierrors.Wrapf(err, "unable to write slot index %d", currentSlot)
		}

		var accountsCount uint64
		if err := pWriter.WriteValue("pf account count", accountsCount, true); err != nil {
			return ierrors.Wrapf(err, "unable to write pf accounts count for slot index %d", currentSlot)
		}

		performanceFactors, err := t.validatorPerformancesFunc(currentSlot)
		if err != nil {
			return ierrors.Wrapf(err, "unable to get performance factors for slot index %d", currentSlot)
		}

		if err = performanceFactors.Stream(func(accountID iotago.AccountID, pf *model.ValidatorPerformance) error {
			if err = pWriter.WriteValue("account id", accountID); err != nil {
				return ierrors.Wrapf(err, "unable to write account id %s for slot %d", accountID, currentSlot)
			}

			bytes, err := t.apiProvider.APIForSlot(currentSlot).Encode(pf)
			if err != nil {
				return ierrors.Wrapf(err, "unable to encode performance factor for accountID %s and slot index %d", accountID, currentSlot)
			}

			if err = pWriter.WriteBytes(bytes); err != nil {
				return ierrors.Wrapf(err, "unable to write performance factor for accountID %s and slot index %d", accountID, currentSlot)
			}

			accountsCount++

			return nil
		}); err != nil {
			return ierrors.Wrapf(err, "unable to write performance factors for slot index %d", currentSlot)
		}

		if err = pWriter.WriteValueAtBookmark("pf account count", accountsCount); err != nil {
			return ierrors.Wrap(err, "unable to write pf accounts count")
		}

		slotCount++
	}

	if err := pWriter.WriteValueAtBookmark("pf slot count", slotCount); err != nil {
		return ierrors.Wrap(err, "unable to write pf slot count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportPoolRewards(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	// export all stored pools
	// in theory we could save the epoch count only once, because stats and rewards should be the same length
	var epochCount uint64
	if err := pWriter.WriteValue("pool rewards epoch count", epochCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write epoch count")
	}

	for epoch := targetEpoch; epoch > iotago.EpochIndex(lo.Max(0, int(targetEpoch)-daysInYear)); epoch-- {
		rewardsMap, err := t.rewardsMap(epoch)
		if err != nil {
			return ierrors.Wrapf(err, "unable to get rewards tree for epoch index %d", epoch)
		}

		// if the map was not present in storage we can skip this epoch and the previous ones, as we never stored any rewards
		if !rewardsMap.WasRestoredFromStorage() {
			break
		}

		if err = pWriter.WriteValue("epoch index", epoch); err != nil {
			return ierrors.Wrapf(err, "unable to write epoch index for epoch index %d", epoch)
		}

		var accountCount uint64
		if err = pWriter.WriteValue("pool rewards account count", accountCount, true); err != nil {
			return ierrors.Wrapf(err, "unable to write account count for epoch index %d", epoch)
		}

		if err = rewardsMap.Stream(func(key iotago.AccountID, value *model.PoolRewards) error {
			if err = pWriter.WriteValue("account id", key); err != nil {
				return ierrors.Wrapf(err, "unable to write account id for epoch index %d and accountID %s", epoch, key)
			}

			if err = pWriter.WriteValue("account rewards", value); err != nil {
				return ierrors.Wrapf(err, "unable to write account rewards for epoch index %d and accountID %s", epoch, key)
			}

			accountCount++

			return nil
		}); err != nil {
			return ierrors.Wrapf(err, "unable to stream rewards for epoch index %d", epoch)
		}

		if err = pWriter.WriteValueAtBookmark("pool rewards account count", accountCount); err != nil {
			return ierrors.Wrapf(err, "unable to write account count for epoch index %d at bookmarked position", epoch)
		}

		epochCount++
	}

	if err := pWriter.WriteValueAtBookmark("pool rewards epoch count", epochCount); err != nil {
		return ierrors.Wrap(err, "unable to write epoch count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportPoolsStats(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("pools stats epoch count", epochCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write epoch count")
	}
	// export all stored pools
	var innerErr error
	if err := t.poolStatsStore.StreamBytes(func(key []byte, value []byte) error {
		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
		if epochIndex > targetEpoch {
			// continue
			return nil
		}
		if err := pWriter.WriteBytes(key); err != nil {
			innerErr = ierrors.Wrapf(err, "unable to write epoch index %d", epochIndex)

			return innerErr
		}

		if err := pWriter.WriteBytes(value); err != nil {
			innerErr = ierrors.Wrapf(err, "unable to write pools stats for epoch %d", epochIndex)

			return innerErr
		}

		epochCount++

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "unable to iterate over pools stats")
	} else if innerErr != nil {
		return ierrors.Wrap(innerErr, "error while iterating over pools stats")
	}
	if err := pWriter.WriteValueAtBookmark("pools stats epoch count", epochCount); err != nil {
		return ierrors.Wrap(err, "unable to write stats epoch count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportCommittees(pWriter *utils.PositionedWriter, targetSlot iotago.SlotIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("committees epoch count", epochCount, true); err != nil {
		return ierrors.Wrap(err, "unable to write committees epoch count")
	}

	apiForSlot := t.apiProvider.APIForSlot(targetSlot)
	epochFromTargetSlot := apiForSlot.TimeProvider().EpochFromSlot(targetSlot)

	pointOfNoReturn := apiForSlot.TimeProvider().EpochEnd(epochFromTargetSlot) - apiForSlot.ProtocolParameters().MaxCommittableAge()

	var innerErr error
	err := t.committeeStore.StreamBytes(func(epochBytes []byte, committeeBytes []byte) error {
		epoch := iotago.EpochIndex(binary.LittleEndian.Uint64(epochBytes))

		// We have a committee for an epoch higher than the targetSlot
		// 1. we trust the point of no return, we export the committee for the next epoch
		// 2. if we don't trust the point-of-no-return
		// - we were able to rotate a committee, then we export it
		// - we were not able to rotate a committee (reused), then we don't export it
		if epoch > epochFromTargetSlot && targetSlot < pointOfNoReturn {
			committee, _, err := account.AccountsFromBytes(committeeBytes)
			if err != nil {
				innerErr = ierrors.Wrapf(err, "failed to parse committee bytes for epoch %d", epoch)

				return innerErr
			}
			if committee.IsReused() {
				return nil
			}
		}

		if err := pWriter.WriteBytes(epochBytes); err != nil {
			innerErr = ierrors.Wrap(err, "unable to write epoch index")

			return innerErr
		}
		if err := pWriter.WriteBytes(committeeBytes); err != nil {
			innerErr = ierrors.Wrap(err, "unable to write epoch committee")

			return innerErr
		}
		epochCount++

		return nil
	})
	if err != nil {
		return ierrors.Wrapf(err, "unable to iterate over committee base store: %w", innerErr)
	}

	if innerErr != nil {
		return ierrors.Wrap(err, "error while iterating over committee base store")
	}

	if err = pWriter.WriteValueAtBookmark("committees epoch count", epochCount); err != nil {
		return ierrors.Wrap(err, "unable to write committee epoch count at bookmarked position")
	}

	return nil
}
