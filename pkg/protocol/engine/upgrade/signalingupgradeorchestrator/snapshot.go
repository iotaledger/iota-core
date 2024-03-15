package signalingupgradeorchestrator

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (o *Orchestrator) Import(reader io.ReadSeeker) error {
	o.evictionMutex.Lock()
	defer o.evictionMutex.Unlock()

	slot, err := stream.Read[iotago.SlotIndex](reader)
	if err != nil {
		return ierrors.Wrap(err, "failed to read last committed slot")
	}
	o.lastCommittedSlot = slot

	upgradeSignalMap := make(map[account.SeatIndex]*model.SignaledBlock)
	//nolint:revive
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		seat, err := stream.Read[account.SeatIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to read seat")
		}

		signaledBlock, err := stream.ReadObjectWithSize(reader, serializer.SeriLengthPrefixTypeAsUint16, model.SignaledBlockFromBytesFunc(o.apiProvider.APIForSlot(slot)))
		if err != nil {
			return ierrors.Wrap(err, "failed to read signaled block")
		}

		upgradeSignalMap[seat] = signaledBlock

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to import upgrade signals for slot %d", slot)
	}

	upgradeSignals, err := o.upgradeSignalsPerSlotFunc(slot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get upgrade signals for slot %d", slot)
	}
	for seat, signaledBlock := range upgradeSignalMap {
		if err := upgradeSignals.Store(seat, signaledBlock); err != nil {
			o.errorHandler(ierrors.Wrapf(err, "failed to store upgrade signals %d:%v", seat, signaledBlock))
			return nil
		}
	}
	// Load latest signals into cache into the next slot (necessary so that we have the correct information when we commit that slot).
	latestSignals := o.latestSignals.Get(slot+1, true)
	for seat, signaledBlock := range upgradeSignalMap {
		latestSignals.Set(seat, signaledBlock)
	}

	//nolint:revive
	if err := stream.ReadCollection(reader, serializer.SeriLengthPrefixTypeAsByte, func(i int) error {
		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to read epoch")
		}

		versionAndHash, err := stream.ReadObject(reader, model.VersionAndHashSize, model.VersionAndHashFromBytes)
		if err != nil {
			return ierrors.Wrap(err, "failed to read versionAndHash")
		}

		if err := o.decidedUpgradeSignals.Store(epoch, versionAndHash); err != nil {
			return ierrors.Wrapf(err, "failed to set permanent upgrade signals for epoch %d", epoch)
		}

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to import permanent upgrade signals for slot %d", slot)
	}

	return nil
}

func (o *Orchestrator) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) error {
	o.evictionMutex.RLock()
	defer o.evictionMutex.RUnlock()

	if err := stream.Write(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to write target slot")
	}

	// Export the upgrade signals for the target slot. Since these are rolled forward exporting the last slot is sufficient.
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		var exportedCount int

		upgradeSignals, err := o.upgradeSignalsPerSlotFunc(targetSlot)
		if err != nil {
			return 0, ierrors.Wrapf(err, "failed to get upgrade signals for target slot %d", targetSlot)
		}
		if err := upgradeSignals.StreamBytes(func(seatBytes []byte, signaledBlockBytes []byte) error {
			if err := stream.WriteBytes(writer, seatBytes); err != nil {
				return ierrors.Wrap(err, "failed to write seat")
			}

			if err := stream.WriteBytesWithSize(writer, signaledBlockBytes, serializer.SeriLengthPrefixTypeAsUint16); err != nil {
				return ierrors.Wrap(err, "failed to write signaled block")
			}

			exportedCount++

			return nil
		}); err != nil {
			return 0, ierrors.Wrapf(err, "failed to stream upgrade signals for target slot %d", targetSlot)
		}

		return exportedCount, nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to write upgrade signals for target slot %d", targetSlot)
	}

	// Export the successfully signaled epochs for the signaling window.
	if err := stream.WriteCollection(writer, serializer.SeriLengthPrefixTypeAsByte, func() (elementsCount int, err error) {
		var exportedCount int

		apiForSlot := o.apiProvider.APIForSlot(targetSlot)
		currentEpoch := apiForSlot.TimeProvider().EpochFromSlot(targetSlot)
		for epoch := o.signalingWindowStart(currentEpoch, apiForSlot); epoch <= currentEpoch; epoch++ {
			versionAndHash, err := o.decidedUpgradeSignals.Load(epoch)
			if err != nil {
				return 0, ierrors.Wrapf(err, "failed to get permanent upgrade signals for epoch %d", epoch)
			}
			if versionAndHash.Version == 0 {
				// We don't write anything to the storage if no supermajority was reached (or no signaling was going on).
				// So it's safe to ignore this here and skip to the next epoch.
				continue
			}

			if err := stream.Write(writer, epoch); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write epoch %d", epoch)
			}
			if err := stream.WriteObject(writer, versionAndHash, model.VersionAndHash.Bytes); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write versionAndHash for epoch %d", epoch)
			}

			exportedCount++
		}

		return exportedCount, nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to write permanent upgrade signals for target slot %d", targetSlot)
	}

	return nil
}
