package signalingupgradeorchestrator

import (
	"github.com/iotaledger/hive.go/core/memstorage"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	"github.com/iotaledger/iota-core/pkg/votes"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Orchestrator is a component that is in charge of protocol upgrades by signaling the upgrade, gathering
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
	evictionMutex     syncutils.RWMutex
	errorHandler      func(error)
	lastCommittedSlot iotago.SlotIndex

	latestSignals             *memstorage.IndexedStorage[iotago.SlotIndex, account.SeatIndex, *model.SignaledBlock]
	upgradeSignalsPerSlotFunc func(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error)
	decidedUpgradeSignals     *epochstore.Store[model.VersionAndHash]

	setProtocolParametersEpochMappingFunc func(iotago.Version, iotago.Identifier, iotago.EpochIndex) error
	protocolParametersAndVersionsHashFunc func() (iotago.Identifier, error)
	epochForVersionFunc                   func(iotago.Version) (iotago.EpochIndex, bool)

	apiProvider iotago.APIProvider
	seatManager seatmanager.SeatManager

	optsProtocolParameters []iotago.ProtocolParameters

	module.Module
}

func NewProvider(opts ...options.Option[Orchestrator]) module.Provider[*engine.Engine, upgrade.Orchestrator] {
	return module.Provide(func(e *engine.Engine) upgrade.Orchestrator {
		o := NewOrchestrator(
			e.ErrorHandler("upgradegadget"),
			e.Storage.DecidedUpgradeSignals(),
			e.Storage.UpgradeSignals,
			e.Storage.Settings().APIProvider(),
			e.Storage.Settings().StoreFutureProtocolParametersHash,
			e.Storage.Settings().APIProvider().VersionsAndProtocolParametersHash,
			e.Storage.Settings().APIProvider().EpochForVersion,
			e.SybilProtection.SeatManager(),
			opts...,
		)

		for _, protocolParams := range o.optsProtocolParameters {
			storedProtocolParams := e.Storage.Settings().APIProvider().ProtocolParameters(protocolParams.Version())
			if storedProtocolParams != nil {
				if lo.PanicOnErr(storedProtocolParams.Hash()) != lo.PanicOnErr(protocolParams.Hash()) {
					panic(ierrors.Errorf("protocol parameters for version %d already exist with different hash", protocolParams.Version()))
				}

				if !storedProtocolParams.Equals(protocolParams) {
					panic(ierrors.Errorf("protocol parameters for version %d already exist but are not equal", protocolParams.Version()))
				}
			}

			if err := e.Storage.Settings().StoreProtocolParameters(protocolParams); err != nil {
				panic(ierrors.Wrapf(err, "failed to store protocol parameters for version %d", protocolParams.Version()))
			}
		}

		o.TriggerInitialized()

		return o
	})
}

func NewOrchestrator(errorHandler func(error),
	decidedUpgradeSignals *epochstore.Store[model.VersionAndHash],
	upgradeSignalsFunc func(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error),
	apiProvider iotago.APIProvider,
	setProtocolParametersEpochMappingFunc func(iotago.Version, iotago.Identifier, iotago.EpochIndex) error,
	protocolParametersAndVersionsHashFunc func() (iotago.Identifier, error),
	epochForVersionFunc func(iotago.Version) (iotago.EpochIndex, bool),
	seatManager seatmanager.SeatManager, opts ...options.Option[Orchestrator]) *Orchestrator {
	return options.Apply(&Orchestrator{
		errorHandler:              errorHandler,
		latestSignals:             memstorage.NewIndexedStorage[iotago.SlotIndex, account.SeatIndex, *model.SignaledBlock](),
		decidedUpgradeSignals:     decidedUpgradeSignals,
		upgradeSignalsPerSlotFunc: upgradeSignalsFunc,

		setProtocolParametersEpochMappingFunc: setProtocolParametersEpochMappingFunc,
		protocolParametersAndVersionsHashFunc: protocolParametersAndVersionsHashFunc,
		epochForVersionFunc:                   epochForVersionFunc,

		apiProvider: apiProvider,
		seatManager: seatManager,
	}, opts,
		(*Orchestrator).TriggerConstructed,
	)
}

func (o *Orchestrator) Shutdown() {
	o.TriggerStopped()
}

func (o *Orchestrator) TrackValidationBlock(block *blocks.Block) {
	// Not a validation block.
	validationBlock, isValidationBlock := block.ValidationBlock()
	if !isValidationBlock {
		return
	}
	newSignaledBlock := model.NewSignaledBlock(block.ID(), block.ProtocolBlock(), validationBlock)

	committee := o.seatManager.Committee(block.ID().Index())
	seat, exists := committee.GetSeat(block.ProtocolBlock().IssuerID)
	if !exists {
		return
	}

	// Do not track any version that we already know about. This includes past, current and future versions that are already
	// successfully signaled and scheduled to start in a future epoch.
	if _, exists := o.epochForVersionFunc(validationBlock.HighestSupportedVersion); exists {
		return
	}

	o.evictionMutex.RLock()
	defer o.evictionMutex.RUnlock()

	latestSignalsForEpoch := o.latestSignals.Get(block.ID().Index(), true)
	o.addNewSignaledBlock(latestSignalsForEpoch, seat, newSignaledBlock)
}

func (o *Orchestrator) addNewSignaledBlock(latestSignalsForEpoch *shrinkingmap.ShrinkingMap[account.SeatIndex, *model.SignaledBlock], seat account.SeatIndex, newSignaledBlock *model.SignaledBlock) {
	latestSignalsForEpoch.Compute(seat, func(currentValue *model.SignaledBlock, exists bool) *model.SignaledBlock {
		if !exists {
			return newSignaledBlock
		}

		if newSignaledBlock.Compare(currentValue) == 1 {
			return newSignaledBlock
		}

		return currentValue
	})
}

func (o *Orchestrator) Commit(slot iotago.SlotIndex) (iotago.Identifier, error) {
	apiForSlot := o.apiProvider.APIForSlot(slot)
	currentEpoch := apiForSlot.TimeProvider().EpochFromSlot(slot)

	lastSlotInEpoch := o.apiProvider.APIForSlot(slot).TimeProvider().EpochEnd(currentEpoch) == slot

	o.evictionMutex.Lock()
	defer o.evictionMutex.Unlock()

	signaledBlockPerSeat := func() map[account.SeatIndex]*model.SignaledBlock {
		// Evict and get latest signals for slot.
		latestSignalsForSlot := o.latestSignals.Evict(slot)
		if latestSignalsForSlot == nil {
			return nil
		}

		signaledBlockPerSeat := latestSignalsForSlot.AsMap()

		// Store upgrade signals for this slot.
		upgradeSignals, err := o.upgradeSignalsPerSlotFunc(slot)
		if err != nil {
			o.errorHandler(ierrors.Wrapf(err, "failed to get upgrade signals for slot %d", slot))
			return nil
		}
		for seat, signaledBlock := range signaledBlockPerSeat {
			if err := upgradeSignals.Store(seat, signaledBlock); err != nil {
				o.errorHandler(ierrors.Wrapf(err, "failed to store upgrade signals %d:%v", seat, signaledBlock))
				return nil
			}
		}

		// Merge latest signals for slot and next slot if next slot is in same epoch. I.e. we carry over the latest signals
		// so that we can check whether the threshold was reached and export based on the latest signals only.
		if !lastSlotInEpoch {
			latestSignalsForNextSlot := o.latestSignals.Get(slot+1, true)
			for seat, signaledBlock := range signaledBlockPerSeat {
				o.addNewSignaledBlock(latestSignalsForNextSlot, seat, signaledBlock)
			}
		}

		o.lastCommittedSlot++

		return signaledBlockPerSeat
	}()

	o.tryUpgrade(currentEpoch, lastSlotInEpoch, signaledBlockPerSeat)

	return o.protocolParametersAndVersionsHashFunc()
}

func (o *Orchestrator) tryUpgrade(currentEpoch iotago.EpochIndex, lastSlotInEpoch bool, signaledBlockPerSeat map[account.SeatIndex]*model.SignaledBlock) {
	// If the threshold was reached in this epoch and this is the last slot of the epoch we want to evaluate whether the window threshold was reached potentially upgrade the version.
	if signaledBlockPerSeat == nil || !lastSlotInEpoch {
		return
	}

	versionAndHashSupporters := make(map[model.VersionAndHash]int)
	for _, signaledBlock := range signaledBlockPerSeat {
		versionAndHash := model.VersionAndHash{
			Version: signaledBlock.HighestSupportedVersion,
			Hash:    signaledBlock.ProtocolParametersHash,
		}
		versionAndHashSupporters[versionAndHash]++
	}

	// Find version with most supporters. Since the threshold is a super-majority we can't have a tie and looking at the
	// version with most supporters is sufficient.
	versionAndHashWithMostSupporters, mostSupporters := o.maxVersionByCount(versionAndHashSupporters)

	// The version has just been activated and will become active in the future. There's no need to continue.
	if _, exists := o.epochForVersionFunc(versionAndHashWithMostSupporters.Version); exists {
		return
	}

	// Check whether the threshold for version was reached.
	totalSeatCount := o.seatManager.SeatCount()
	if !votes.IsThresholdReached(mostSupporters, totalSeatCount, votes.SuperMajority) {
		return
	}

	// Store information that threshold for version was reached for this epoch.
	if err := o.decidedUpgradeSignals.Store(currentEpoch, versionAndHashWithMostSupporters); err != nil {
		o.errorHandler(ierrors.Wrap(err, "failed to store permanent upgrade signals"))
		return
	}

	// Check whether the signaling window threshold is reached.
	versionAndHashTobeUpgraded, reached := o.signalingThresholdReached(currentEpoch)
	if !reached {
		return
	}

	// The version should be upgraded. We're adding the version to the settings.
	// Effectively, this is a soft fork as it is contained in the hash of protocol parameters and versions.
	if err := o.setProtocolParametersEpochMappingFunc(versionAndHashTobeUpgraded.Version, versionAndHashTobeUpgraded.Hash, currentEpoch+iotago.EpochIndex(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().ActivationOffset)); err != nil {
		o.errorHandler(ierrors.Wrap(err, "failed to set protocol parameters epoch mapping"))
		return
	}
}

func (o *Orchestrator) maxVersionByCount(versionSupporters map[model.VersionAndHash]int) (model.VersionAndHash, int) {
	var versionWithMostSupporters model.VersionAndHash
	var mostSupporters int
	for versionAndHash, supportersCount := range versionSupporters {
		if supportersCount > mostSupporters {
			versionWithMostSupporters = versionAndHash
			mostSupporters = supportersCount
		}
	}

	return versionWithMostSupporters, mostSupporters
}

func (o *Orchestrator) signalingThresholdReached(currentEpoch iotago.EpochIndex) (model.VersionAndHash, bool) {
	epochVersions := make(map[model.VersionAndHash]int)

	for epoch := o.signalingWindowStart(currentEpoch); epoch <= currentEpoch; epoch++ {
		version, err := o.decidedUpgradeSignals.Load(epoch)
		if err != nil {
			o.errorHandler(ierrors.Wrapf(err, "failed to get permanent upgrade signals for epoch %d in %d", epoch, currentEpoch))

			return model.VersionAndHash{}, false
		}
		if version.Version == 0 {
			continue
		}

		epochVersions[version]++
	}

	// Find version with that was most signaled in the signalingWindow. Since the WindowTargetRatio
	// is a super-majority we can't have a tie and looking at the version with most supporters is sufficient.
	versionMostSignaled, signaledCount := o.maxVersionByCount(epochVersions)

	// Check whether the signaling window threshold is reached.
	if signaledCount < int(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().WindowTargetRatio) {
		return model.VersionAndHash{}, false
	}

	return versionMostSignaled, true
}

func (o *Orchestrator) signalingWindowStart(epoch iotago.EpochIndex) iotago.EpochIndex {
	windowSize := iotago.EpochIndex(o.apiProvider.CurrentAPI().ProtocolParameters().VersionSignaling().WindowSize)

	if epoch < windowSize {
		return 0
	}

	return epoch - windowSize
}
