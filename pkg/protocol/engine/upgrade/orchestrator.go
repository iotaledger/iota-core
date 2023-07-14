package upgrade

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
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Orchestrator is a software component that is in charge of protocol upgrades by signaling the upgrade, gathering
// consensus about the upgrade and finally activating the upgrade if the activation thresholds are reached.
//
// Why is it called Orchestrator? Read on...
//
// User
// how would you name a software component that is in charge of protocol upgrades by signaling the upgrade, gathering
// consensus about the upgrade and then activating the upgrade.
//
// ChatGPT
// One possible name for such a software component could be the "Upgrade Manager." This name conveys its responsibility
// of managing protocol upgrades within a system or network. The Upgrade Manager would handle tasks like signaling,
// consensus gathering, and activation of upgrades, ensuring smooth transitions and maintaining the integrity of the protocol.
//
// User
// do you have a less boring name? everything else is also called manager
//
// ChatGPT
// Certainly! If you're looking for a more creative and less generic name, you could consider calling it the "Protocol Phoenix."
// This name captures the essence of the component's role in orchestrating protocol upgrades, as the mythical phoenix
// symbolizes rebirth and renewal. It signifies the component's ability to facilitate the evolution and transformation
// of the protocol, rising from the ashes of the previous version.
//
// User
// maybe a more technical and less mythical term
//
// ChatGPT
// Certainly! In that case, you could consider naming the software component the "Upgrade Orchestrator."
// This name emphasizes its role in orchestrating protocol upgrades, managing the various stages of the process, and
// ensuring a smooth transition. The term "orchestrator" suggests a conductor-like function, coordinating the upgrade
// activities and ensuring consensus before activating the changes.
type Orchestrator struct {
	evictionMutex syncutils.RWMutex
	errorHandler  func(error)

	latestSignals           *memstorage.IndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock]
	upgradeSignalsFunc      func(slot iotago.SlotIndex) *prunable.UpgradeSignals
	permanentUpgradeSignals *kvstore.TypedStore[iotago.EpochIndex, iotago.Version]

	setProtocolParametersEpochMappingFunc func(iotago.Version, iotago.EpochIndex) error
	protocolParametersAndVersionsHashFunc func(iotago.SlotIndex) (iotago.Identifier, error)
	epochForVersionFunc                   func(iotago.Version) (iotago.EpochIndex, bool)

	apiProvider api.Provider
	seatManager seatmanager.SeatManager

	module.Module
}

func NewProvider(opts ...options.Option[Orchestrator]) module.Provider[*engine.Engine, *Orchestrator] {
	return module.Provide(func(e *engine.Engine) *Orchestrator {
		o := NewOrchestrator(
			e.ErrorHandler("upgradegadget"),
			e.Storage.Permanent.UpgradeSignals(),
			e.Storage.Prunable.UpgradeSignals,
			e.Storage.Settings(),
			e.Storage.Settings().StoreProtocolParametersEpochMapping,
			e.Storage.Settings().VersionsAndProtocolParametersHash,
			e.Storage.Settings().EpochForVersion,
			e.SybilProtection.SeatManager(),
			opts...,
		)

		e.Events.BlockGadget.BlockAccepted.Hook(o.trackHighestSupportedVersion)
		o.TriggerInitialized()

		return o
	})
}

func NewOrchestrator(errorHandler func(error),
	permanentUpgradeSignal kvstore.KVStore,
	upgradeSignalsFunc func(slot iotago.SlotIndex) *prunable.UpgradeSignals,
	apiProvider api.Provider,
	setProtocolParametersEpochMappingFunc func(iotago.Version, iotago.EpochIndex) error,
	protocolParametersAndVersionsHashFunc func(iotago.SlotIndex) (iotago.Identifier, error),
	epochForVersionFunc func(iotago.Version) (iotago.EpochIndex, bool),
	seatManager seatmanager.SeatManager, opts ...options.Option[Orchestrator]) *Orchestrator {
	return options.Apply(&Orchestrator{
		errorHandler:            errorHandler,
		latestSignals:           memstorage.NewIndexedStorage[iotago.SlotIndex, account.SeatIndex, *prunable.SignalledBlock](),
		permanentUpgradeSignals: kvstore.NewTypedStore[iotago.EpochIndex, iotago.Version](permanentUpgradeSignal, iotago.EpochIndex.Bytes, iotago.EpochIndexFromBytes, iotago.Version.Bytes, iotago.VersionFromBytes),
		upgradeSignalsFunc:      upgradeSignalsFunc,

		setProtocolParametersEpochMappingFunc: setProtocolParametersEpochMappingFunc,
		protocolParametersAndVersionsHashFunc: protocolParametersAndVersionsHashFunc,
		epochForVersionFunc:                   epochForVersionFunc,

		apiProvider: apiProvider,
		seatManager: seatManager,
	}, opts,
		(*Orchestrator).TriggerConstructed,
	)
}

// TODO
//  - when creating snapshot we need to export slot the stuff on disk of the latest committed slot. starting from disk should be fine
//  - when starting from disk/snapshot we need to fill up latestSignals
//  - add option to NewProvider: slice of protocol parameters -> write to settings if not existent, panic if different

func (o *Orchestrator) Shutdown() {
	o.TriggerStopped()
}

func (o *Orchestrator) trackHighestSupportedVersion(block *blocks.Block) {
	committee := o.seatManager.Committee(block.ID().Index())
	seat, exists := committee.GetSeat(block.ProtocolBlock().IssuerID)
	if !exists {
		return
	}

	// Not a validation block.
	newSignalledBlock, err := prunable.NewSignalledBlock(block.ID(), block.ProtocolBlock())
	if err != nil {
		return
	}

	// Do not track any version that we already know about. This includes past, current and future versions that are already
	// successfully signaled and scheduled to start in a future epoch.
	if _, exists := o.epochForVersionFunc(block.ProtocolBlock().ProtocolVersion); exists {
		return
	}

	// TODO: track version+hash of protocol parameters as there could be the same version with different protocol parameters

	o.evictionMutex.RLock()
	defer o.evictionMutex.RUnlock()

	latestSignalsForEpoch := o.latestSignals.Get(block.ID().Index(), true)
	o.addNewSignalledBlock(latestSignalsForEpoch, seat, newSignalledBlock)
}

func (o *Orchestrator) addNewSignalledBlock(latestSignalsForEpoch *shrinkingmap.ShrinkingMap[account.SeatIndex, *prunable.SignalledBlock], seat account.SeatIndex, newSignalledBlock *prunable.SignalledBlock) {
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

func (o *Orchestrator) Commit(slot iotago.SlotIndex) (iotago.Identifier, error) {
	apiForSlot := o.apiProvider.APIForSlot(slot)
	currentEpoch := apiForSlot.TimeProvider().EpochFromSlot(slot)

	lastSlotInEpoch := o.apiProvider.APIForSlot(slot).TimeProvider().EpochEnd(currentEpoch) == slot

	signalledBlockPerSeat := func() map[account.SeatIndex]*prunable.SignalledBlock {
		o.evictionMutex.Lock()
		defer o.evictionMutex.Unlock()

		// Evict and get latest signals for slot.
		latestSignalsForSlot := o.latestSignals.Evict(slot)
		if latestSignalsForSlot == nil {
			return nil
		}

		signalledBlockPerSeat := latestSignalsForSlot.AsMap()

		// Store upgrade signals for this slot.
		upgradeSignals := o.upgradeSignalsFunc(slot)
		if err := upgradeSignals.Store(signalledBlockPerSeat); err != nil {
			o.errorHandler(ierrors.Wrap(err, "failed to store upgrade signals"))
			return nil
		}

		// Merge latest signals for slot and next slot if next slot is in same epoch. I.e. we carry over the latest signals
		// so that we can check whether the threshold was reached and export based on the latest signals only.
		if !lastSlotInEpoch {
			latestSignalsForNextSlot := o.latestSignals.Get(slot+1, true)
			for seat, signalledBlock := range signalledBlockPerSeat {
				o.addNewSignalledBlock(latestSignalsForNextSlot, seat, signalledBlock)
			}
		}

		return signalledBlockPerSeat
	}()

	o.tryUpgrade(currentEpoch, lastSlotInEpoch, signalledBlockPerSeat)

	return o.protocolParametersAndVersionsHashFunc(slot)
}

func (o *Orchestrator) tryUpgrade(currentEpoch iotago.EpochIndex, lastSlotInEpoch bool, signalledBlockPerSeat map[account.SeatIndex]*prunable.SignalledBlock) {
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
	versionWithMostSupporters, mostSupporters := o.maxVersionByCount(versionSupporters)

	// Check whether the threshold for version was reached.
	totalSeatCount := o.seatManager.SeatCount()
	if !votes.IsThresholdReached(mostSupporters, totalSeatCount, votes.SuperMajority) {
		return
	}

	// Store information that threshold for version was reached for this epoch.
	if err := o.permanentUpgradeSignals.Set(currentEpoch, versionWithMostSupporters); err != nil {
		o.errorHandler(ierrors.Wrap(err, "failed to store permanent upgrade signals"))
		return
	}

	// Check whether the signaling window threshold is reached.
	versionTobeUpgraded, reached := o.signalingThresholdReached(currentEpoch)
	if !reached {
		return
	}

	// The version should be upgraded. We're adding the version to the settings.
	// Effectively, this is a soft fork as it is contained in the hash of protocol parameters and versions.
	if err := o.setProtocolParametersEpochMappingFunc(versionTobeUpgraded, currentEpoch+iotago.EpochIndex(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().ActivationOffset)); err != nil {
		o.errorHandler(ierrors.Wrap(err, "failed to set protocol parameters epoch mapping"))
		return
	}
}

func (o *Orchestrator) maxVersionByCount(versionSupporters map[iotago.Version]int) (iotago.Version, int) {
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

func (o *Orchestrator) signalingThresholdReached(currentEpoch iotago.EpochIndex) (iotago.Version, bool) {
	epochVersions := make(map[iotago.Version]int)

	for epoch := o.signallingWindowStart(currentEpoch); epoch <= currentEpoch; epoch++ {
		version, err := o.permanentUpgradeSignals.Get(epoch)
		if err != nil {
			if ierrors.Is(err, kvstore.ErrKeyNotFound) {
				continue
			}

			o.errorHandler(ierrors.Wrap(err, "failed to get permanent upgrade signals"))

			return 0, false
		}

		epochVersions[version]++
	}

	// Find version with that was most signaled in the signalingWindow. Since the optsVersionSignallingWindowThreshold
	// is a super-majority we can't have a tie and looking at the version with most supporters is sufficient.
	versionMostSignaled, signaledCount := o.maxVersionByCount(epochVersions)

	// Check whether the signaling window threshold is reached.
	if signaledCount < int(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().WindowTargetRatio) {
		return 0, false
	}

	return versionMostSignaled, true
}

func (o *Orchestrator) signallingWindowStart(epoch iotago.EpochIndex) iotago.EpochIndex {
	windowSize := iotago.EpochIndex(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().WindowSize)

	if epoch < windowSize {
		return 0
	}

	return epoch - windowSize
}
