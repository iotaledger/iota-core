package performance

//
//func (t *Tracker) Rollback(targetSlotIndex iotago.SlotIndex) error {
//	t.mutex.Lock()
//	defer t.mutex.Unlock()
//
//	timeProvider := t.apiProvider.APIForSlot(targetSlotIndex).TimeProvider()
//	targetEpoch := timeProvider.EpochFromSlot(targetSlotIndex)
//
//	// if the target index is the last slot of the epoch, the epoch was committed
//	if timeProvider.EpochEnd(targetEpoch) != targetSlotIndex {
//		targetEpoch--
//	}
//
//	err := t.rollbackPerformanceFactor(timeProvider.EpochStart(targetEpoch+1), targetSlotIndex)
//	if err != nil {
//		return ierrors.Wrap(err, "unable to export performance factor")
//	}
//
//	err = t.rollbackPoolRewards(targetEpoch)
//	if err != nil {
//		return ierrors.Wrap(err, "unable to export pool rewards")
//	}
//
//	err = t.rollbackPoolsStats(targetEpoch)
//	if err != nil {
//		return ierrors.Wrap(err, "unable to export pool stats")
//	}
//
//	err = t.rollbackCommittees(targetSlotIndex)
//	if err != nil {
//		return ierrors.Wrap(err, "unable to export committees")
//	}
//
//	return nil
//}
//
//func (t *Tracker) rollbackPerformanceFactor(targetSlot, lastCommittedSlot iotago.SlotIndex) error {
//	t.performanceFactorsMutex.RLock()
//	defer t.performanceFactorsMutex.RUnlock()
//
//	for currentSlot := targetSlot; currentSlot <= targetSlot; currentSlot++ {
//		// TODO: decrease this in import/export to uint16 in pf Load/Store/... if we are sure on the performance factor calculation and its expected upper bond
//		// TODO: clean the current epoch only as future epochs will be removed on disk
//		performanceFactors, err := t.performanceFactorsFunc(currentSlot)
//		if err != nil {
//			return ierrors.Wrapf(err, "unable to get performance factors for slot index %d", currentSlot)
//		}
//
//	}
//
//	return nil
//}
//
//func (t *Tracker) rollbackPoolRewards(targetEpoch, lastCommittedEpoch iotago.EpochIndex) error {
//
//	for epoch := targetEpoch + 1; epoch <= lastCommittedEpoch; epoch++ {
//		rewardsMap, err := t.rewardsStorePerEpochFunc(epoch)
//		if err != nil {
//			return ierrors.Wrapf(err, "unable to get rewards store for epoch index %d", epoch)
//		}
//
//		if err := rewardsMap.Clear(); err != nil {
//			return ierrors.Wrapf(err, "error while clearing rewards store for epoch %d", epoch)
//		}
//	}
//
//	return nil
//}
//
//func (t *Tracker) rollbackPoolsStats(targetEpoch iotago.EpochIndex) error {
//	var epochCount uint64
//	if err := pWriter.WriteValue("pools stats epoch count", epochCount, true); err != nil {
//		return ierrors.Wrap(err, "unable to write epoch count")
//	}
//	// export all stored pools
//	var innerErr error
//	if err := t.poolStatsStore.StreamBytes(func(key []byte, value []byte) error {
//		epochIndex := iotago.EpochIndex(binary.LittleEndian.Uint64(key))
//		if epochIndex > targetEpoch {
//			// continue
//			return nil
//		}
//		if err := pWriter.WriteBytes(key); err != nil {
//			innerErr = ierrors.Wrapf(err, "unable to write epoch index %d", epochIndex)
//			return innerErr
//		}
//		if err := pWriter.WriteBytes(value); err != nil {
//			innerErr = ierrors.Wrapf(err, "unable to write pools stats for epoch %d", epochIndex)
//			return innerErr
//		}
//		epochCount++
//
//		return nil
//	}); err != nil {
//		return ierrors.Wrap(err, "unable to iterate over pools stats")
//	} else if innerErr != nil {
//		return ierrors.Wrap(innerErr, "error while iterating over pools stats")
//	}
//	if err := pWriter.WriteValueAtBookmark("pools stats epoch count", epochCount); err != nil {
//		return ierrors.Wrap(err, "unable to write stats epoch count at bookmarked position")
//	}
//
//	return nil
//}
//
//func (t *Tracker) rollbackCommittees(targetSlot iotago.SlotIndex) error {
//	var epochCount uint64
//	if err := pWriter.WriteValue("committees epoch count", epochCount, true); err != nil {
//		return ierrors.Wrap(err, "unable to write committees epoch count")
//	}
//	apiForSlot := t.apiProvider.APIForSlot(targetSlot)
//	epochFromTargetSlot := apiForSlot.TimeProvider().EpochFromSlot(targetSlot)
//
//	pointOfNoReturn := apiForSlot.TimeProvider().EpochEnd(epochFromTargetSlot) - apiForSlot.ProtocolParameters().MaxCommittableAge()
//
//	var innerErr error
//	err := t.committeeStore.StreamBytes(func(epochBytes []byte, committeeBytes []byte) error {
//		epoch := iotago.EpochIndex(binary.LittleEndian.Uint64(epochBytes))
//
//		// We have a committee for an epoch higher than the targetSlot
//		// 1. we trust the point of no return, we export the committee for the next epoch
//		// 2. if we don't trust the point-of-no-return
//		// - we were able to rotate a committee, then we export it
//		// - we were not able to rotate a committee (reused), then we don't export it
//		if epoch > epochFromTargetSlot && targetSlot < pointOfNoReturn {
//			committee, _, err := account.AccountsFromBytes(committeeBytes)
//			if err != nil {
//				innerErr = ierrors.Wrapf(err, "failed to parse committee bytes for epoch %d", epoch)
//				return innerErr
//			}
//			if committee.IsReused() {
//				return nil
//			}
//		}
//
//		if err := pWriter.WriteBytes(epochBytes); err != nil {
//			innerErr = ierrors.Wrap(err, "unable to write epoch index")
//			return innerErr
//		}
//		if err := pWriter.WriteBytes(committeeBytes); err != nil {
//			innerErr = ierrors.Wrap(err, "unable to write epoch committee")
//			return innerErr
//		}
//		epochCount++
//
//		return nil
//	})
//	if err != nil {
//		return ierrors.Wrapf(err, "unable to iterate over committee base store: %w", innerErr)
//	}
//	if innerErr != nil {
//		return ierrors.Wrap(err, "error while iterating over committee base store")
//	}
//	if err = pWriter.WriteValueAtBookmark("committees epoch count", epochCount); err != nil {
//		return ierrors.Wrap(err, "unable to write committee epoch count at bookmarked position")
//	}
//
//	return nil
//}
