package signalingupgradeorchestrator

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
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
	if err := stream.ReadCollection(reader, func(i int) error {
		seat, err := stream.Read[account.SeatIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to read seat")
		}

		signaledBlock, err := stream.ReadFunc(reader, model.SignaledBlockFromBytesFunc(o.apiProvider.APIForSlot(slot)))
		if err != nil {
			return ierrors.Wrap(err, "failed to read signaled block")
		}

		upgradeSignalMap[seat] = signaledBlock

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "failed to import upgrade signals for slot %d", slot)
	}

	upgradeSignals := o.upgradeSignalsFunc(slot)
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

	if err := stream.ReadCollection(reader, func(i int) error {
		epoch, err := stream.Read[iotago.EpochIndex](reader)
		if err != nil {
			return ierrors.Wrap(err, "failed to read epoch")
		}

		versionAndHash, err := stream.ReadFunc(reader, VersionAndHashFromBytes)
		if err != nil {
			return ierrors.Wrap(err, "failed to read versionAndHash")
		}

		if err := o.permanentUpgradeSignals.Set(epoch, versionAndHash); err != nil {
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
	if err := stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		var exportedCount uint64

		upgradeSignals := o.upgradeSignalsFunc(targetSlot)
		if err := upgradeSignals.StreamBytes(func(seatBytes []byte, signaledBlockBytes []byte) error {
			if err := stream.Write(writer, seatBytes); err != nil {
				return ierrors.Wrap(err, "failed to write seat")
			}

			if err := stream.WriteBlob(writer, signaledBlockBytes); err != nil {
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
	if err := stream.WriteCollection(writer, func() (elementsCount uint64, err error) {
		var exportedCount uint64

		currentEpoch := o.apiProvider.CurrentAPI().TimeProvider().EpochFromSlot(targetSlot)
		for epoch := o.signalingWindowStart(currentEpoch); epoch <= currentEpoch; epoch++ {
			versionAndHash, err := o.permanentUpgradeSignals.Get(epoch)
			if err != nil {
				// We don't write anything to the storage if no supermajority was reached (or no signaling was going on).
				// So it's safe to ignore this here and skip to the next epoch.
				if ierrors.Is(err, kvstore.ErrKeyNotFound) {
					continue
				}

				if err != nil {
					return 0, ierrors.Wrapf(err, "failed to get permanent upgrade signals for epoch %d", epoch)
				}
			}

			if err := stream.Write(writer, epoch); err != nil {
				return 0, ierrors.Wrapf(err, "failed to write epoch %d", epoch)
			}
			if err := stream.WriteSerializable(writer, versionAndHash); err != nil {
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
