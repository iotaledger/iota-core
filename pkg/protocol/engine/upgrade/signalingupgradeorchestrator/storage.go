package signalingupgradeorchestrator

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (o *Orchestrator) RestoreFromDisk(slot iotago.SlotIndex) error {
	o.evictionMutex.Lock()
	defer o.evictionMutex.Unlock()

	o.lastCommittedSlot = slot

	// Load latest signals into cache into the next slot (necessary so that we have the correct information when we commit that slot).
	latestSignals := o.latestSignals.Get(slot+1, true)
	upgradeSignals, err := o.upgradeSignalsPerSlotFunc(slot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get upgrade signals for slot %d", slot)
	}
	if err := upgradeSignals.Stream(func(seat account.SeatIndex, signaledBlock *model.SignaledBlock) error {
		latestSignals.Set(seat, signaledBlock)

		return nil
	}); err != nil {
		return ierrors.Wrap(err, "failed to restore upgrade signals from disk")
	}

	return nil
}
