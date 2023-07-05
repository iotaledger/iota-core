package performance

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (t *Tracker) Import(reader io.ReadSeeker) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if err := t.importPerformanceFactor(reader); err != nil {
		return errors.Wrap(err, "unable to import performance factor")
	}

	if err := t.importPoolRewards(reader); err != nil {
		return errors.Wrap(err, "unable to import pool rewards")
	}

	if err := t.importPoolsStats(reader); err != nil {
		return errors.Wrap(err, "unable to import pool stats")
	}

	if err := t.importCommittees(reader); err != nil {
		return errors.Wrap(err, "unable to import committees")
	}

	return nil
}

func (t *Tracker) Export(writer io.WriteSeeker, targetSlotIndex iotago.SlotIndex) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	targetEpoch := t.timeProvider.EpochFromSlot(targetSlotIndex)
	positionedWriter := utils.NewPositionedWriter(writer)

	// if the target index is the last slot of the epoch, the epoch was committed
	if t.timeProvider.EpochEnd(targetEpoch) != targetSlotIndex {
		targetEpoch--
	}

	err := t.exportPerformanceFactor(positionedWriter, t.timeProvider.EpochStart(targetEpoch+1), targetSlotIndex)
	if err != nil {
		return errors.Wrap(err, "unable to export performance factor")
	}

	err = t.exportPoolRewards(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool rewards")
	}

	err = t.exportPoolsStats(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export pool stats")
	}

	err = t.exportCommittees(positionedWriter, targetEpoch)
	if err != nil {
		return errors.Wrap(err, "unable to export committees")
	}

	return nil
}

func (t *Tracker) importPerformanceFactor(reader io.ReadSeeker) error {
	var slotCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &slotCount); err != nil {
		return errors.Wrap(err, "unable to read slot count")
	}

	for i := uint64(0); i < slotCount; i++ {
		var slotIndex iotago.SlotIndex
		if err := binary.Read(reader, binary.LittleEndian, &slotIndex); err != nil {
			return errors.Wrap(err, "unable to read slot index")
		}

		// TODO: if accounts count is limited to the committee size, we can lower this to uint8 or other matching value
		var accountsCount uint64
		if err := binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return errors.Wrapf(err, "unable to read accounts count for slot index %d", slotIndex)
		}

		performanceFactors := t.performanceFactorsFunc(slotIndex)
		for j := uint64(0); j < accountsCount; j++ {
			var accountID iotago.AccountID
			if err := binary.Read(reader, binary.LittleEndian, &accountID); err != nil {
				return errors.Wrapf(err, "unable to read account id for the slot index %d", slotIndex)
			}
			// TODO: decrease this in import/export to uint16 in pf Load/Store/... if we are sure on the performance factor calculation and its expected upper bond
			var performanceFactor uint64
			if err := binary.Read(reader, binary.LittleEndian, &performanceFactor); err != nil {
				return errors.Wrapf(err, "unable to read performance factor for account %s and slot index %d", accountID.String(), slotIndex)
			}
			err := performanceFactors.Store(accountID, performanceFactor)
			if err != nil {
				return errors.Wrapf(err, "unable to store performance factor for account %s and slot index %d", accountID.String(), slotIndex)
			}
		}
	}

	return nil
}

func (t *Tracker) importPoolRewards(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return errors.Wrap(err, "unable to read epoch count")
	}

	for i := uint64(0); i < epochCount; i++ {
		var epochIndex iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epochIndex); err != nil {
			return errors.Wrap(err, "unable to read epoch index")
		}

		rewardsTree := ads.NewMap[iotago.AccountID, PoolRewards](t.rewardsStorage(epochIndex))

		var accountsCount uint64
		if err := binary.Read(reader, binary.LittleEndian, &accountsCount); err != nil {
			return errors.Wrapf(err, "unable to read accounts count for epoch index %d", epochIndex)
		}

		for j := uint64(0); j < accountsCount; j++ {
			var accountID iotago.AccountID
			if err := binary.Read(reader, binary.LittleEndian, &accountID); err != nil {
				return errors.Wrapf(err, "unable to read account id for the epoch index %d", epochIndex)
			}

			var reward PoolRewards
			if err := binary.Read(reader, binary.LittleEndian, &reward); err != nil {
				return errors.Wrapf(err, "unable to read reward for account %s and epoch index %d", accountID.String(), epochIndex)
			}
			rewardsTree.Set(accountID, &reward)
		}
	}

	return nil
}

func (t *Tracker) importPoolsStats(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return errors.Wrap(err, "unable to read epoch count")
	}
	for i := uint64(0); i < epochCount; i++ {
		var epochIndex iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epochIndex); err != nil {
			return errors.Wrap(err, "unable to read epoch index")
		}

		var poolStats PoolsStats
		if err := binary.Read(reader, binary.LittleEndian, &poolStats); err != nil {
			return errors.Wrapf(err, "unable to read pool stats for epoch index %d", epochIndex)
		}

		err := t.poolStatsStore.Set(epochIndex.Bytes(), lo.PanicOnErr(poolStats.Bytes()))
		if err != nil {
			return errors.Wrapf(err, "unable to store pool stats for the epoch index %d", epochIndex)
		}
	}

	return nil
}

func (t *Tracker) importCommittees(reader io.ReadSeeker) error {
	var epochCount uint64
	if err := binary.Read(reader, binary.LittleEndian, &epochCount); err != nil {
		return errors.Wrap(err, "unable to read committees epoch count")
	}
	for i := uint64(0); i < epochCount; i++ {
		var epoch iotago.EpochIndex
		if err := binary.Read(reader, binary.LittleEndian, &epoch); err != nil {
			return errors.Wrap(err, "unable to read epoch index")
		}

		committee := account.NewAccounts()
		if err := committee.FromReader(reader); err != nil {
			return errors.Wrapf(err, "unable to read committee for the epoch index %d", epoch)
		}

		err := t.committeeStore.Set(epoch.Bytes(), lo.PanicOnErr(committee.Bytes()))
		if err != nil {
			return errors.Wrapf(err, "unable to store committee for the epoch index %d", epoch)
		}
	}

	return nil
}

func (t *Tracker) exportPerformanceFactor(pWriter *utils.PositionedWriter, startSlot, targetSlot iotago.SlotIndex) error {
	t.performanceFactorsMutex.RLock()
	defer t.performanceFactorsMutex.RUnlock()

	var slotCount uint64
	if err := pWriter.WriteValue("pf slot count", slotCount, true); err != nil {
		return errors.Wrap(err, "unable to write pf slot count")
	}

	for currentSlot := startSlot; currentSlot <= targetSlot; currentSlot++ {
		if err := pWriter.WriteValue("slot index", currentSlot); err != nil {
			return errors.Wrapf(err, "unable to write slot index %d", currentSlot)
		}
		// TODO: if accounts count is limited to the committee size, we can lower this to uint8 or other matching value
		var accountsCount uint64
		if err := pWriter.WriteValue("pf account count", accountsCount, true); err != nil {
			return errors.Wrapf(err, "unable to write pf accounts count for slot index %d", currentSlot)
		}
		// TODO: decrease this in import/export to uint16 in pf Load/Store/... if we are sure on the performance factor calculation and its expected upper bond
		if err := t.performanceFactorsFunc(currentSlot).ForEachPerformanceFactor(func(accountID iotago.AccountID, pf uint64) error {
			if err := pWriter.WriteValue("account id", accountID); err != nil {
				return errors.Wrapf(err, "unable to write account id %s for slot %d", accountID, currentSlot)
			}
			if err := pWriter.WriteValue("performance factor", pf); err != nil {
				return errors.Wrapf(err, "unable to write performance factor for account ID %s and slot index %d", accountID, currentSlot)
			}
			accountsCount++

			return nil
		}); err != nil {
			return errors.Wrapf(err, "unable to write performance factors for slot index %d", currentSlot)
		}

		if err := pWriter.WriteValueAtBookmark("pf account count", accountsCount); err != nil {
			return errors.Wrap(err, "unable to write pf accounts count")
		}

		slotCount++
	}

	if err := pWriter.WriteValueAtBookmark("pf slot count", slotCount); err != nil {
		return errors.Wrap(err, "unable to write pf slot count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportPoolRewards(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	// export all stored pools
	// in theory we could save the epoch count only once, because stats and rewards should be the same length
	var epochCount uint64
	if err := pWriter.WriteValue("pool rewards epoch count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write epoch count")
	}
	// TODO: make sure to adjust this loop if we add pruning later.
	for epoch := targetEpoch; epoch > iotago.EpochIndex(0); epoch-- {
		rewardsTree := ads.NewMap[iotago.AccountID, PoolRewards](t.rewardsStorage(epoch))

		// if the tree is new, we can skip this epoch and the previous ones, as we never stored any rewards
		if rewardsTree.IsNew() {
			break
		}

		if err := pWriter.WriteValue("epoch index", epoch); err != nil {
			return errors.Wrapf(err, "unable to write epoch index for epoch index %d", epoch)
		}

		var accountCount uint64
		if err := pWriter.WriteValue("pool rewards account count", accountCount, true); err != nil {
			return errors.Wrapf(err, "unable to write account count for epoch index %d", epoch)
		}

		var innerErr error
		if err := rewardsTree.Stream(func(key iotago.AccountID, value *PoolRewards) bool {
			if err := pWriter.WriteValue("account id", key); err != nil {
				innerErr = errors.Wrapf(err, "unable to write account id for epoch index %d and accountID %s", epoch, key)
				return false
			}
			if err := pWriter.WriteValue("account rewards", value); err != nil {
				innerErr = errors.Wrapf(err, "unable to write account rewards for epoch index %d and accountID %s", epoch, key)
				return false
			}
			accountCount++

			return true
		}); err != nil {
			return errors.Wrapf(err, "unable to stream rewards for epoch index %d", epoch)
		}
		if innerErr != nil {
			return innerErr
		}

		if err := pWriter.WriteValueAtBookmark("pool rewards account count", accountCount); err != nil {
			return errors.Wrapf(err, "unable to write account count for epoch index %d at bookmarked position", epoch)
		}

		epochCount++
	}

	if err := pWriter.WriteValueAtBookmark("pool rewards epoch count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write epoch count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportPoolsStats(pWriter *utils.PositionedWriter, targetEpoch iotago.EpochIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("pools stats epoch count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write epoch count")
	}
	// export all stored pools
	var innerErr error
	if err := t.poolStatsStore.Iterate([]byte{}, func(key []byte, value []byte) bool {
		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
		if epochIndex > targetEpoch {
			// continue
			return true
		}
		if err := pWriter.WriteBytes(key); err != nil {
			innerErr = errors.Wrapf(err, "unable to write epoch index %d", epochIndex)
			return false
		}
		if err := pWriter.WriteBytes(value); err != nil {
			innerErr = errors.Wrapf(err, "unable to write pools stats for epoch %d", epochIndex)
			return false
		}
		epochCount++

		return true
	}); err != nil {
		return errors.Wrap(err, "unable to iterate over pools stats")
	} else if innerErr != nil {
		return errors.Wrap(innerErr, "error while iterating over pools stats")
	}
	if err := pWriter.WriteValueAtBookmark("pools stats epoch count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write stats epoch count at bookmarked position")
	}

	return nil
}

func (t *Tracker) exportCommittees(pWriter *utils.PositionedWriter, _ iotago.EpochIndex) error {
	var epochCount uint64
	if err := pWriter.WriteValue("committees epoch count", epochCount, true); err != nil {
		return errors.Wrap(err, "unable to write committees epoch count")
	}

	var innerErr error
	err := t.committeeStore.Iterate([]byte{}, func(epochBytes []byte, committeeBytes []byte) bool {
		// TODO: committees for all available epochs should be included in the snapshot,
		//  because if snapshot is generated after the committee has been selected,
		//  then there is no way for the node to determine the committee.
		//  There will be at most one epoch more in the store than targetEpoch,
		//  so the question is whether we should explicitly make sure to only include one more committee,
		//  or should we explicitly limit that? Currently we use the implicit assumption.
		//epoch := iotago.EpochIndex(binary.LittleEndian.Uint64(epochBytes))
		//if epoch > targetEpoch {
		//	return true
		//}

		if err := pWriter.WriteBytes(epochBytes); err != nil {
			innerErr = errors.Wrap(err, "unable to write epoch index")
			return false
		}
		if err := pWriter.WriteBytes(committeeBytes); err != nil {
			innerErr = errors.Wrap(err, "unable to write epoch committee")
			return false
		}
		epochCount++

		return true
	})
	if err != nil {
		return errors.Wrapf(err, "unable to iterate over committee base store: %s", innerErr)
	}
	if innerErr != nil {
		return errors.Wrapf(err, "error while iterating over committee base store")
	}
	if err = pWriter.WriteValueAtBookmark("committees epoch count", epochCount); err != nil {
		return errors.Wrap(err, "unable to write committee epoch count at bookmarked position")
	}

	return nil
}
