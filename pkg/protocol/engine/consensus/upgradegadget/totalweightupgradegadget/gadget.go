package totalweightupgradegadget

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
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

	latestSignals           *memstorage.IndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock]
	upgradeSignalsFunc      func(slot iotago.SlotIndex) *prunable.UpgradeSignals
	permanentUpgradeSignals *kvstore.TypedStore[iotago.EpochIndex, iotago.Version]

	setProtocolParametersEpochMappingFunc func(iotago.Version, iotago.EpochIndex) error
	protocolParametersAndVersionsHashFunc func() (iotago.Identifier, error)

	apiProvider api.Provider
	seatManager seatmanager.SeatManager

	optsVersionSignallingThreshold       float64
	optsVersionSignallingWindowSize      iotago.EpochIndex
	optsVersionSignallingWindowThreshold int
	optsNewVersionStartOffset            iotago.EpochIndex

	module.Module
}

func NewProvider(opts ...options.Option[Gadget]) module.Provider[*engine.Engine, upgradegadget.Gadget] {
	return module.Provide(func(e *engine.Engine) upgradegadget.Gadget {
		return options.Apply(&Gadget{
			optsVersionSignallingThreshold:       0.67,
			optsVersionSignallingWindowSize:      7,
			optsVersionSignallingWindowThreshold: 5,
			optsNewVersionStartOffset:            7,
			errorHandler:                         e.ErrorHandler("upgradegadget"),
			latestSignals:                        memstorage.NewIndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock](),
		}, opts, func(g *Gadget) {
			g.upgradeSignalsFunc = e.Storage.Prunable.UpgradeSignals
			g.permanentUpgradeSignals = kvstore.NewTypedStore[iotago.EpochIndex, iotago.Version](e.Storage.Permanent.UpgradeSignals(), iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes, iotago.Version.Bytes, iotago.VersionFromBytes)
			g.apiProvider = e.Storage.Settings()
			g.setProtocolParametersEpochMappingFunc = e.Storage.Settings().StoreProtocolParametersEpochMapping
			g.protocolParametersAndVersionsHashFunc = e.Storage.Settings().ProtocolParametersAndVersionsHash

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

func (g *Gadget) Commit(slot iotago.SlotIndex) (iotago.Identifier, error) {
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

	g.tryUpgrade(currentEpoch, lastSlotInEpoch, signalledBlockPerSeat)

	return g.protocolParametersAndVersionsHashFunc()
}

func (g *Gadget) tryUpgrade(currentEpoch iotago.EpochIndex, lastSlotInEpoch bool, signalledBlockPerSeat map[account.SeatIndex]*prunable.SignalledBlock) {
	// If the threshold was reached in this epoch and this is the last slot of the epoch we want to evaluate whether the window threshold was reached potentially upgrade the version.
	if signalledBlockPerSeat == nil || !lastSlotInEpoch {
		return
	}

	versionSupporters := make(map[iotago.Version]int)
	for _, signalledBlock := range signalledBlockPerSeat {
		versionSupporters[signalledBlock.HighestSupportedVersion]++
	}

	// Find version with most supporters. Since the threshold is a super-majority we can't have a tie and looking at the
	// version with most supporters is sufficient.
	versionWithMostSupporters, mostSupporters := g.mostSupportedVersion(versionSupporters)

	// Check whether the threshold for version was reached.
	totalSeatCount := g.seatManager.SeatCount()
	if !votes.IsThresholdReached(mostSupporters, totalSeatCount, g.optsVersionSignallingThreshold) {
		return
	}

	// Store information that threshold for version was reached for this epoch.
	if err := g.permanentUpgradeSignals.Set(currentEpoch, versionWithMostSupporters); err != nil {
		g.errorHandler(ierrors.Wrap(err, "failed to store permanent upgrade signals"))
		return
	}

	// Check whether the signalling window threshold is reached.
	versionTobeUpgraded, reached := g.signallingThresholdReached(currentEpoch)
	if !reached {
		return
	}

	// The version should be upgraded. We're adding the version to the settings.
	// Effectively, this is a soft fork as it is contained in the hash of protocol parameters and versions.
	if err := g.setProtocolParametersEpochMappingFunc(versionTobeUpgraded, currentEpoch+g.optsNewVersionStartOffset); err != nil {
		g.errorHandler(ierrors.Wrap(err, "failed to set protocol parameters epoch mapping"))
		return
	}
}

func (g *Gadget) mostSupportedVersion(versionSupporters map[iotago.Version]int) (iotago.Version, int) {
	var versionWithMostSupporters iotago.Version
	var mostSupporters int
	for version, supportersCount := range versionSupporters {
		if supportersCount > mostSupporters {
			versionWithMostSupporters = version
			mostSupporters = supportersCount
		}
	}

	return versionWithMostSupporters, mostSupporters
}

func (g *Gadget) signallingThresholdReached(currentEpoch iotago.EpochIndex) (iotago.Version, bool) {
	epochVersions := make(map[iotago.Version]int)

	for epoch := g.signallingWindowStart(currentEpoch); epoch <= currentEpoch; epoch++ {
		version, err := g.permanentUpgradeSignals.Get(epoch)
		if err != nil {
			if ierrors.Is(err, kvstore.ErrKeyNotFound) {
				continue
			}

			g.errorHandler(ierrors.Wrap(err, "failed to get permanent upgrade signals"))

			return 0, false
		}

		epochVersions[version]++
	}

	// Find version with most supporters. Since the optsVersionSignallingWindowThreshold is a super-majority we can't
	// have a tie and looking at the version with most supporters is sufficient.
	versionWithMostSupporters, mostSupporters := g.mostSupportedVersion(epochVersions)

	// Check whether the signalling window threshold is reached.
	if mostSupporters < g.optsVersionSignallingWindowThreshold {
		return 0, false
	}

	return versionWithMostSupporters, true
}

func (g *Gadget) signallingWindowStart(epoch iotago.EpochIndex) iotago.EpochIndex {
	if epoch < g.optsVersionSignallingWindowSize {
		return 0
	}

	return epoch - g.optsVersionSignallingWindowSize
}

var _ upgradegadget.Gadget = new(Gadget)
