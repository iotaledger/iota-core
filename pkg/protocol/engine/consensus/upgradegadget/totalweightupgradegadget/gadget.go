package totalweightupgradegadget

import (
	"fmt"

	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/api"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/upgradegadget"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Gadget struct {
	evictionMutex syncutils.RWMutex
	errorHandler  func(error)

	latestSignals      *memstorage.IndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock]
	upgradeSignalsFunc func(slot iotago.SlotIndex) *prunable.UpgradeSignals

	apiProvider api.Provider
	seatManager seatmanager.SeatManager

	optsVersionSignallingThreshold float64
	optsVersionSignallingWindow    iotago.EpochIndex
	optsNewVersionStartOffset      iotago.EpochIndex

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, upgradegadget.Gadget] {
	return module.Provide(func(e *engine.Engine) upgradegadget.Gadget {
		return options.Apply(&Gadget{
			optsVersionSignallingThreshold: 0.67,
			optsVersionSignallingWindow:    7,
			optsNewVersionStartOffset:      7,
			errorHandler:                   e.ErrorHandler("upgradegadget"),
			latestSignals:                  memstorage.NewIndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock](),
		}, opts, func(g *Gadget) {
			g.upgradeSignalsFunc = e.Storage.UpgradeSignals
			g.apiProvider = e.Storage.Settings()

			e.Events.BlockGadget.BlockAccepted.Hook(g.trackHighestSupportedVersion)

			// TODO: fill options from protocol parameters
		},
			(*Gadget).TriggerConstructed,
			(*Gadget).TriggerInitialized,
		)
	})
}

// TODO
//  - when creating snapshot we need to export slot the stuff on disk of the latest committed slot. starting from disk should be fine
//  - when starting from disk/snapshot we need to fill up latestSignals

func (g *Gadget) Shutdown() {
	g.TriggerStopped()
}

func (g *Gadget) trackHighestSupportedVersion(block *blocks.Block) {
	committee := g.seatManager.Committee(block.ID().Index())
	seat, exists := committee.GetSeat(block.ProtocolBlock().IssuerID)
	if !exists {
		return
	}

	// Not a validation block.
	newSignalledBlock, err := prunable.NewSignalledBlock(block.ID(), block.ProtocolBlock())
	if err != nil {
		return
	}

	// TODO:
	//  1. do we want to track the highest supported version also if it's the same as the current version?
	// 	2. can a validator vote on 2 versions at the same time? does a vote on a higher version include a vote on a lower version?
	//  	- can we skip a version?
	// 		- are there going to be parallel votes on different versions going on at the same time?

	// TODO: check whether hash of protocol parameters is the same

	g.evictionMutex.RLock()
	defer g.evictionMutex.RUnlock()

	latestSignalsForEpoch := g.latestSignals.Get(block.ID().Index(), true)
	g.addNewSignalledBlock(latestSignalsForEpoch, seat, newSignalledBlock)
}

func (g *Gadget) addNewSignalledBlock(latestSignalsForEpoch *shrinkingmap.ShrinkingMap[account.SeatIndex, *prunable.SignalledBlock], seat account.SeatIndex, newSignalledBlock *prunable.SignalledBlock) {
	latestSignalsForEpoch.Compute(seat, func(currentValue *prunable.SignalledBlock, exists bool) *prunable.SignalledBlock {
		if !exists {
			return newSignalledBlock
		}

		if newSignalledBlock.Compare(currentValue) == 1 {
			return newSignalledBlock
		}

		return currentValue
	})
}

func (g *Gadget) Commit(slot iotago.SlotIndex) {
	apiForSlot := g.apiProvider.APIForSlot(slot)
	currentEpoch := apiForSlot.TimeProvider().EpochFromSlot(slot)

	lastSlotInEpoch := g.apiProvider.APIForSlot(slot).TimeProvider().EpochEnd(currentEpoch) == slot

	signalledBlockPerSeat := func() map[account.SeatIndex]*prunable.SignalledBlock {
		g.evictionMutex.Lock()
		defer g.evictionMutex.Unlock()

		// Evict and get latest signals for slot.
		latestSignalsForSlot := g.latestSignals.Evict(slot)
		if latestSignalsForSlot == nil {
			return nil
		}

		signalledBlockPerSeat := latestSignalsForSlot.AsMap()

		// Store upgrade signals for this slot.
		upgradeSignals := g.upgradeSignalsFunc(slot)
		if err := upgradeSignals.Store(signalledBlockPerSeat); err != nil {
			g.errorHandler(ierrors.Wrap(err, "failed to store upgrade signals"))
			return nil
		}

		// Merge latest signals for slot and next slot if next slot is in same epoch. I.e. we carry over the latest signals
		// so that we can check whether the threshold was reached and export based on the latest signals only.
		if !lastSlotInEpoch {
			latestSignalsForNextSlot := g.latestSignals.Get(slot+1, true)
			for seat, signalledBlock := range signalledBlockPerSeat {
				g.addNewSignalledBlock(latestSignalsForNextSlot, seat, signalledBlock)
			}
		}

		return signalledBlockPerSeat
	}()

	if signalledBlockPerSeat == nil || !lastSlotInEpoch {
		return
	}

	// TODO: When this is the last slot of the epoch we want to evaluate whether the threshold was reached and export the version.

	versionSupporters := make(map[iotago.Version][]account.SeatIndex)
	for seat, signalledBlock := range signalledBlockPerSeat {
		versionSupporters[signalledBlock.HighestSupportedVersion] = append(versionSupporters[signalledBlock.HighestSupportedVersion], seat)
	}

	totalSeatCount := g.seatManager.SeatCount()
	for version, supporters := range versionSupporters {
		if votes.IsThresholdReached(len(supporters), totalSeatCount, g.optsVersionSignallingThreshold) {
			// TODO: write version to permanent storage
			fmt.Println("threshold reached for version", version, "with supporters", supporters)

			//	- compress information whether threshold reached
			//	- check whether threshold was reached 5/7 times before
			//	-> if yes then add version number
		}

	}
}

var _ upgradegadget.Gadget = new(Gadget)
