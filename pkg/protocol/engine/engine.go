package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/syncmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/upgrade"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/retainer"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Engine /////////////////////////////////////////////////////////////////////////////////////////////////////

type Engine struct {
	Events              *Events
	Storage             *storage.Storage
	Filter              filter.Filter
	CommitmentFilter    commitmentfilter.CommitmentFilter
	EvictionState       *eviction.State
	BlockRequester      *eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]
	BlockDAG            blockdag.BlockDAG
	Booker              booker.Booker
	Clock               clock.Clock
	BlockGadget         blockgadget.Gadget
	SlotGadget          slotgadget.Gadget
	SybilProtection     sybilprotection.SybilProtection
	Notarization        notarization.Notarization
	Attestations        attestation.Attestations
	Ledger              ledger.Ledger
	Scheduler           scheduler.Scheduler
	TipManager          tipmanager.TipManager
	TipSelection        tipselection.TipSelection
	Retainer            retainer.Retainer
	SyncManager         syncmanager.SyncManager
	UpgradeOrchestrator upgrade.Orchestrator
	RootCommitment      reactive.Variable[*model.Commitment]
	LatestCommitment    reactive.Variable[*model.Commitment]

	Workers      *workerpool.Group
	errorHandler func(error)

	BlockCache *blocks.Blocks

	chainID iotago.CommitmentID
	mutex   syncutils.RWMutex

	optsSnapshotPath     string
	optsEntryPointsDepth int
	optsSnapshotDepth    int
	optsBlockRequester   []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]]

	*module.ReactiveModule
}

func New(
	logger log.Logger,
	workers *workerpool.Group,
	storageInstance *storage.Storage,
	filterProvider module.Provider[*Engine, filter.Filter],
	commitmentFilterProvider module.Provider[*Engine, commitmentfilter.CommitmentFilter],
	blockDAGProvider module.Provider[*Engine, blockdag.BlockDAG],
	bookerProvider module.Provider[*Engine, booker.Booker],
	clockProvider module.Provider[*Engine, clock.Clock],
	blockGadgetProvider module.Provider[*Engine, blockgadget.Gadget],
	slotGadgetProvider module.Provider[*Engine, slotgadget.Gadget],
	sybilProtectionProvider module.Provider[*Engine, sybilprotection.SybilProtection],
	notarizationProvider module.Provider[*Engine, notarization.Notarization],
	attestationProvider module.Provider[*Engine, attestation.Attestations],
	ledgerProvider module.Provider[*Engine, ledger.Ledger],
	schedulerProvider module.Provider[*Engine, scheduler.Scheduler],
	tipManagerProvider module.Provider[*Engine, tipmanager.TipManager],
	tipSelectionProvider module.Provider[*Engine, tipselection.TipSelection],
	retainerProvider module.Provider[*Engine, retainer.Retainer],
	upgradeOrchestratorProvider module.Provider[*Engine, upgrade.Orchestrator],
	syncManagerProvider module.Provider[*Engine, syncmanager.SyncManager],
	opts ...options.Option[Engine],
) (engine *Engine) {
	var importSnapshot bool
	var file *os.File
	var fileErr error

	return options.Apply(
		&Engine{
			Events:           NewEvents(),
			Storage:          storageInstance,
			EvictionState:    eviction.NewState(storageInstance.LatestNonEmptySlot(), storageInstance.RootBlocks),
			RootCommitment:   reactive.NewVariable[*model.Commitment](),
			LatestCommitment: reactive.NewVariable[*model.Commitment](),
			Workers:          workers,

			optsSnapshotPath:  "snapshot.bin",
			optsSnapshotDepth: 5,
		}, opts, func(e *Engine) {
			e.ReactiveModule = e.initReactiveModule(logger)

			e.errorHandler = func(err error) {
				e.LogTrace("engine error", "err", err)
			}

			// Import the settings from the snapshot file if needed.
			if importSnapshot = !e.Storage.Settings().IsSnapshotImported() && e.optsSnapshotPath != ""; importSnapshot {
				file, fileErr = os.Open(e.optsSnapshotPath)
				if fileErr != nil {
					panic(ierrors.Wrap(fileErr, "failed to open snapshot file"))
				}

				if err := e.ImportSettings(file); err != nil {
					panic(ierrors.Wrap(err, "failed to import snapshot settings"))
				}
			}
		},
		func(e *Engine) {
			// setup reactive variables
			e.initRootCommitment()
			e.initLatestCommitment()

			// setup all components
			e.BlockCache = blocks.New(e.EvictionState, e.Storage.Settings().APIProvider())
			e.BlockRequester = eventticker.New(e.optsBlockRequester...)
			e.SybilProtection = sybilProtectionProvider(e)
			e.BlockDAG = blockDAGProvider(e)
			e.Filter = filterProvider(e)
			e.CommitmentFilter = commitmentFilterProvider(e)
			e.Booker = bookerProvider(e)
			e.Clock = clockProvider(e)
			e.BlockGadget = blockGadgetProvider(e)
			e.SlotGadget = slotGadgetProvider(e)
			e.Notarization = notarizationProvider(e)
			e.Attestations = attestationProvider(e)
			e.Ledger = ledgerProvider(e)
			e.TipManager = tipManagerProvider(e)
			e.Scheduler = schedulerProvider(e)
			e.TipSelection = tipSelectionProvider(e)
			e.Retainer = retainerProvider(e)
			e.UpgradeOrchestrator = upgradeOrchestratorProvider(e)
			e.SyncManager = syncManagerProvider(e)
		},
		(*Engine).setupBlockStorage,
		(*Engine).setupEvictionState,
		(*Engine).setupBlockRequester,
		(*Engine).setupPruning,
		(*Engine).acceptanceHandler,
		func(e *Engine) {
			e.Constructed.Trigger()

			// Make sure that we have the protocol parameters for the latest supported iota.go protocol version of the software.
			// If not the user needs to update the protocol parameters file.
			// This can only happen after a user updated the node version and the new protocol version is not yet active.
			if _, err := e.APIForVersion(iotago.LatestProtocolVersion()); err != nil {
				panic(ierrors.Wrap(err, "no protocol parameters for latest protocol version found"))
			}

			// Import the rest of the snapshot if needed.
			if importSnapshot {
				if err := e.ImportContents(file); err != nil {
					panic(ierrors.Wrap(err, "failed to import snapshot contents"))
				}

				if closeErr := file.Close(); closeErr != nil {
					panic(closeErr)
				}

				// Only mark any pruning indexes if we loaded a non-genesis snapshot
				if e.Storage.Settings().LatestFinalizedSlot() > 0 {
					if _, _, err := e.Storage.PruneByDepth(1); err != nil {
						if !ierrors.Is(err, database.ErrNoPruningNeeded) &&
							!ierrors.Is(err, database.ErrEpochPruned) {
							panic(ierrors.Wrap(err, "failed to prune storage"))
						}
					}
				}

				if err := e.Storage.Settings().SetSnapshotImported(); err != nil {
					panic(ierrors.Wrap(err, "failed to set snapshot imported"))
				}

			} else {
				// Restore from Disk
				e.Storage.RestoreFromDisk()
				e.EvictionState.PopulateFromStorage(e.Storage.Settings().LatestCommitment().Slot())

				if err := e.Attestations.RestoreFromDisk(); err != nil {
					panic(ierrors.Wrap(err, "failed to restore attestations from disk"))
				}
				if err := e.UpgradeOrchestrator.RestoreFromDisk(e.Storage.Settings().LatestCommitment().Slot()); err != nil {
					panic(ierrors.Wrap(err, "failed to restore upgrade orchestrator from disk"))
				}
			}

			e.Reset()

			e.Initialized.Trigger()
		},
	)
}

func (e *Engine) ProcessBlockFromPeer(block *model.Block, source peer.ID) {
	e.Filter.ProcessReceivedBlock(block, source)
	e.Events.BlockProcessed.Trigger(block.ID())
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (e *Engine) Reset() {
	e.LogDebug("resetting engine", "slot", e.Storage.Settings().LatestCommitment().Slot(), "stack-trace", debug.StackTrace(false, 0))

	e.BlockRequester.Clear()
	e.Storage.Reset()
	e.EvictionState.Reset()
	e.Filter.Reset()
	e.CommitmentFilter.Reset()
	e.BlockCache.Reset()
	e.BlockDAG.Reset()
	e.Booker.Reset()
	e.Ledger.Reset()
	e.BlockGadget.Reset()
	e.SlotGadget.Reset()
	e.Notarization.Reset()
	e.Attestations.Reset()
	e.SybilProtection.Reset()
	e.Scheduler.Reset()
	e.TipManager.Reset()
	e.TipSelection.Reset()
	e.Retainer.Reset()
	e.SyncManager.Reset()
	e.UpgradeOrchestrator.Reset()

	latestCommittedSlot := e.Storage.Settings().LatestCommitment().Slot()
	latestCommittedTime := e.APIForSlot(latestCommittedSlot).TimeProvider().SlotEndTime(latestCommittedSlot)
	e.Clock.Reset(latestCommittedTime)
}

func (e *Engine) shutdown() {
	e.BlockRequester.Shutdown()
	e.Attestations.Shutdown()
	e.SyncManager.Shutdown()
	e.Notarization.Shutdown()
	e.Booker.Shutdown()
	e.Ledger.Shutdown()
	e.BlockDAG.Shutdown()
	e.BlockGadget.Shutdown()
	e.SlotGadget.Shutdown()
	e.Clock.Shutdown()
	e.SybilProtection.Shutdown()
	e.UpgradeOrchestrator.Shutdown()
	e.TipManager.Shutdown()
	e.Filter.Shutdown()
	e.CommitmentFilter.Shutdown()
	e.Scheduler.Shutdown()
	e.Retainer.Shutdown()
	e.Workers.Shutdown()
	e.Storage.Shutdown()

	e.Stopped.Trigger()
}

func (e *Engine) BlockFromCache(id iotago.BlockID) (*blocks.Block, bool) {
	return e.BlockCache.Block(id)
}

func (e *Engine) Block(id iotago.BlockID) (*model.Block, bool) {
	cachedBlock, exists := e.BlockCache.Block(id)
	if exists && !cachedBlock.IsRootBlock() {
		return cachedBlock.ModelBlock(), !cachedBlock.IsMissing()
	}

	s, err := e.Storage.Blocks(id.Slot())
	if err != nil {
		e.errorHandler(ierrors.Wrap(err, "failed to get block storage"))

		return nil, false
	}

	modelBlock, err := s.Load(id)
	if err != nil {
		e.errorHandler(ierrors.Wrap(err, "failed to load block from storage"))

		return nil, false
	}

	return modelBlock, modelBlock != nil
}

func (e *Engine) CommittedAPI() iotago.API {
	return e.Storage.Settings().APIProvider().CommittedAPI()
}

func (e *Engine) APIForTime(t time.Time) iotago.API {
	return e.Storage.Settings().APIProvider().APIForTime(t)
}

func (e *Engine) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return e.Storage.Settings().APIProvider().APIForSlot(slot)
}

func (e *Engine) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return e.Storage.Settings().APIProvider().APIForEpoch(epoch)
}

func (e *Engine) APIForVersion(version iotago.Version) (iotago.API, error) {
	return e.Storage.Settings().APIProvider().APIForVersion(version)
}

func (e *Engine) LatestAPI() iotago.API {
	return e.Storage.Settings().APIProvider().LatestAPI()
}

// CommittedSlot returns the committed slot for the given slot index.
func (e *Engine) CommittedSlot(commitmentID iotago.CommitmentID) (*CommittedSlotAPI, error) {
	if e.Storage.Settings().LatestCommitment().Slot() < commitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", commitmentID.Slot())
	}

	return NewCommittedSlotAPI(e, commitmentID), nil
}

func (e *Engine) WriteSnapshot(filePath string, targetSlot ...iotago.SlotIndex) (err error) {
	if len(targetSlot) == 0 {
		targetSlot = append(targetSlot, e.Storage.Settings().LatestCommitment().Slot())
	} else if lastPrunedEpoch, hasPruned := e.Storage.LastPrunedEpoch(); hasPruned && e.APIForSlot(targetSlot[0]).TimeProvider().EpochFromSlot(targetSlot[0]) <= lastPrunedEpoch {
		return ierrors.Errorf("impossible to create a snapshot for slot %d because it is pruned (last pruned slot %d)", targetSlot[0], lo.Return1(e.Storage.LastPrunedEpoch()))
	}

	if fileHandle, err := os.Create(filePath); err != nil {
		return ierrors.Wrap(err, "failed to create snapshot file")
	} else if err = e.Export(fileHandle, targetSlot[0]); err != nil {
		return ierrors.Wrap(err, "failed to write snapshot")
	} else if err = fileHandle.Close(); err != nil {
		return ierrors.Wrap(err, "failed to close snapshot file")
	}

	return
}

func (e *Engine) ImportSettings(reader io.ReadSeeker) (err error) {
	if err = e.Storage.Settings().Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import settings")
	}

	return
}

func (e *Engine) ImportContents(reader io.ReadSeeker) (err error) {
	if err = e.Storage.Commitments().Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import commitments")
	} else if err = e.Ledger.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import ledger")
	} else if err := e.SybilProtection.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import sybil protection")
	} else if err = e.EvictionState.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import eviction state")
	} else if err = e.Attestations.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import attestation state")
	} else if err = e.UpgradeOrchestrator.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import upgrade orchestrator")
	}

	return
}

func (e *Engine) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	targetCommitment, err := e.Storage.Commitments().Load(targetSlot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to load target commitment at slot %d", targetSlot)
	}

	if err = e.Storage.Settings().Export(writer, targetCommitment.Commitment()); err != nil {
		return ierrors.Wrap(err, "failed to export settings")
	} else if err = e.Storage.Commitments().Export(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to export commitments")
	} else if err = e.Ledger.Export(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to export ledger")
	} else if err := e.SybilProtection.Export(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to export sybil protection")
	} else if err = e.EvictionState.Export(writer, e.Storage.Settings().LatestFinalizedSlot(), targetSlot); err != nil {
		// The rootcommitment is determined from the rootblocks. Therefore, we need to export starting from the last finalized slot.
		return ierrors.Wrap(err, "failed to export eviction state")
	} else if err = e.Attestations.Export(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to export attestation state")
	} else if err = e.UpgradeOrchestrator.Export(writer, targetSlot); err != nil {
		return ierrors.Wrap(err, "failed to export upgrade orchestrator")
	}

	return
}

// RemoveFromFilesystem removes the directory of the engine from the filesystem.
func (e *Engine) RemoveFromFilesystem() error {
	return os.RemoveAll(e.Storage.Directory())
}

func (e *Engine) Name() string {
	return filepath.Base(e.Storage.Directory())
}

func (e *Engine) ChainID() iotago.CommitmentID {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.chainID
}

func (e *Engine) SetChainID(chainID iotago.CommitmentID) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.chainID = chainID
}

func (e *Engine) acceptanceHandler() {
	wp := e.Workers.CreatePool("BlockAccepted", workerpool.WithWorkerCount(1))

	e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		e.Ledger.TrackBlock(block)
		e.SybilProtection.TrackValidationBlock(block)
		e.UpgradeOrchestrator.TrackValidationBlock(block)

		e.Events.AcceptedBlockProcessed.Trigger(block)
	}, event.WithWorkerPool(wp))
}

func (e *Engine) setupBlockStorage() {
	wp := e.Workers.CreatePool("BlockStorage", workerpool.WithWorkerCount(1)) // Using just 1 worker to avoid contention

	e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		store, err := e.Storage.Blocks(block.ID().Slot())
		if err != nil {
			e.errorHandler(ierrors.Errorf("failed to store block with %s, storage with given index does not exist", block.ID()))
			return
		}

		if err := store.Store(block.ModelBlock()); err != nil {
			e.errorHandler(ierrors.Wrapf(err, "failed to store block with %s", block.ID()))
		}
	}, event.WithWorkerPool(wp))
}

func (e *Engine) setupEvictionState() {
	e.Events.EvictionState.LinkTo(e.EvictionState.Events)

	wp := e.Workers.CreatePool("EvictionState", workerpool.WithWorkerCount(1)) // Using just 1 worker to avoid contention

	e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		block.ForEachParent(func(parent iotago.Parent) {
			if parent.ID.Slot() < block.ID().Slot() && !e.EvictionState.IsRootBlock(parent.ID) {
				parentBlock, exists := e.Block(parent.ID)
				if !exists {
					e.errorHandler(ierrors.Errorf("cannot store root block (%s) because it is missing", parent.ID))
					return
				}
				e.EvictionState.AddRootBlock(parentBlock.ID(), parentBlock.ProtocolBlock().SlotCommitmentID)
			}
		})
	}, event.WithWorkerPool(wp))

	e.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		e.EvictionState.AdvanceActiveWindowToIndex(commitment.Slot())
	}, event.WithWorkerPool(wp))

	e.Events.EvictionState.SlotEvicted.Hook(e.BlockCache.EvictUntil)

	e.EvictionState.Initialize(e.Storage.Settings().LatestCommitment().Slot())
}

func (e *Engine) setupBlockRequester() {
	e.Events.BlockRequester.LinkTo(e.BlockRequester.Events)

	e.Events.EvictionState.SlotEvicted.Hook(e.BlockRequester.EvictUntil)

	// We need to hook to make sure that the request is created before the block arrives to avoid a race condition
	// where we try to delete the request again before it is created. Thus, continuing to request forever.
	e.Events.BlockDAG.BlockMissing.Hook(func(block *blocks.Block) {
		e.BlockRequester.StartTicker(block.ID())
	})
	e.Events.BlockDAG.MissingBlockAttached.Hook(func(block *blocks.Block) {
		e.BlockRequester.StopTicker(block.ID())
	}, event.WithWorkerPool(e.Workers.CreatePool("BlockRequester", workerpool.WithWorkerCount(1)))) // Using just 1 worker to avoid contention
}

func (e *Engine) setupPruning() {
	e.Events.SlotGadget.SlotFinalized.Hook(func(slot iotago.SlotIndex) {
		if err := e.Storage.TryPrune(); err != nil {
			e.errorHandler(ierrors.Wrapf(err, "failed to prune storage at slot %d", slot))
		}
	}, event.WithWorkerPool(e.Workers.CreatePool("PruneEngine", workerpool.WithWorkerCount(1))))
}

// EarliestRootCommitment is used to make sure that the chainManager knows the earliest possible
// commitment that blocks we are solidifying will refer to. Failing to do so will prevent those blocks
// from being processed as their chain will be deemed unsolid.
// lastFinalizedSlot is needed to make sure that the root commitment is not younger than the last finalized slot.
// If setting the root commitment based on the last evicted slot this basically means we won't be able to solidify another
// chain beyond a window based on eviction, which in turn is based on acceptance. In case of a partition, this behavior is
// clearly not desired.
func (e *Engine) EarliestRootCommitment(lastFinalizedSlot iotago.SlotIndex) (earliestCommitment *model.Commitment) {
	maxCommittableAge := e.APIForSlot(lastFinalizedSlot).ProtocolParameters().MaxCommittableAge()

	var earliestRootCommitmentSlot iotago.SlotIndex
	if lastFinalizedSlot <= maxCommittableAge {
		earliestRootCommitmentSlot = 0
	} else {
		earliestRootCommitmentSlot = lastFinalizedSlot - maxCommittableAge
	}

	rootCommitment, err := e.Storage.Commitments().Load(earliestRootCommitmentSlot)
	if err != nil {
		panic(fmt.Sprintf("could not load earliest commitment %d after engine initialization: %s", earliestRootCommitmentSlot, err))
	}

	return rootCommitment
}

func (e *Engine) ErrorHandler(componentName string) func(error) {
	return func(err error) {
		e.errorHandler(ierrors.Wrap(err, componentName))
	}
}

func (e *Engine) initRootCommitment() {
	updateRootCommitment := func(lastFinalizedSlot iotago.SlotIndex) {
		maxCommittableAge := e.APIForSlot(lastFinalizedSlot).ProtocolParameters().MaxCommittableAge()

		e.RootCommitment.Compute(func(rootCommitment *model.Commitment) *model.Commitment {
			return lo.Return1(e.Storage.Commitments().Load(lo.Cond(lastFinalizedSlot < maxCommittableAge, 0, lastFinalizedSlot-maxCommittableAge)))
		})
	}

	e.Constructed.OnTrigger(func() {
		unsubscribe := e.Events.SlotGadget.SlotFinalized.Hook(updateRootCommitment).Unhook

		e.Initialized.OnTrigger(func() {
			updateRootCommitment(e.Storage.Settings().LatestFinalizedSlot())
		})

		e.Shutdown.OnTrigger(unsubscribe)
	})
}

func (e *Engine) initLatestCommitment() {
	updateLatestCommitment := func(latestCommitment *model.Commitment) {
		e.LatestCommitment.Compute(func(currentLatestComponent *model.Commitment) *model.Commitment {
			return lo.Cond(currentLatestComponent == nil || currentLatestComponent.Slot() < latestCommitment.Slot(), latestCommitment, currentLatestComponent)
		})
	}

	e.Constructed.OnTrigger(func() {
		unsubscribe := e.Events.Notarization.LatestCommitmentUpdated.Hook(updateLatestCommitment).Unhook

		e.Initialized.OnTrigger(func() {
			updateLatestCommitment(e.Storage.Settings().LatestCommitment())
		})

		e.Shutdown.OnTrigger(unsubscribe)
	})
}

func (e *Engine) initReactiveModule(logger log.Logger) (reactiveModule *module.ReactiveModule) {
	stopLogging := reactive.NewEvent()

	reactiveModule = module.NewReactiveModule(logger.NewEntityLogger("Engine", stopLogging, func(engineLogger log.Logger) {
		// TODO: setup reactive logging
	}))

	reactiveModule.Shutdown.OnTrigger(func() {
		stopLogging.Trigger()

		e.shutdown()
	})

	return reactiveModule
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Options //////////////////////////////////////////////////////////////////////////////////////////////////////

func WithSnapshotPath(snapshotPath string) options.Option[Engine] {
	return func(e *Engine) {
		e.optsSnapshotPath = snapshotPath
	}
}

func WithEntryPointsDepth(entryPointsDepth int) options.Option[Engine] {
	return func(engine *Engine) {
		engine.optsEntryPointsDepth = entryPointsDepth
	}
}

func WithSnapshotDepth(depth int) options.Option[Engine] {
	return func(e *Engine) {
		e.optsSnapshotDepth = depth
	}
}

func WithBlockRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]]) options.Option[Engine] {
	return func(e *Engine) {
		e.optsBlockRequester = append(e.optsBlockRequester, opts...)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
