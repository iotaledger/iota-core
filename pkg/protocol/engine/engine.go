package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Engine /////////////////////////////////////////////////////////////////////////////////////////////////////

type Engine struct {
	Events          *Events
	Storage         *storage.Storage
	Filter          filter.Filter
	EvictionState   *eviction.State
	BlockRequester  *eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]
	BlockDAG        blockdag.BlockDAG
	Booker          booker.Booker
	Clock           clock.Clock
	SybilProtection sybilprotection.SybilProtection
	BlockGadget     blockgadget.Gadget
	SlotGadget      slotgadget.Gadget
	Notarization    notarization.Notarization
	Attestations    attestation.Attestations
	Ledger          ledger.Ledger
	TipManager      tipmanager.TipManager

	Workers      *workerpool.Group
	errorHandler func(error)

	BlockCache *blocks.Blocks

	isBootstrapped      bool
	isBootstrappedMutex sync.Mutex

	chainID iotago.CommitmentID
	mutex   sync.RWMutex

	optsBootstrappedThreshold time.Duration
	optsEntryPointsDepth      int
	optsSnapshotDepth         int
	optsBlockRequester        []options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]]

	module.Module
}

func New(
	workers *workerpool.Group,
	errorHandler func(error),
	storageInstance *storage.Storage,
	filterProvider module.Provider[*Engine, filter.Filter],
	blockDAGProvider module.Provider[*Engine, blockdag.BlockDAG],
	bookerProvider module.Provider[*Engine, booker.Booker],
	clockProvider module.Provider[*Engine, clock.Clock],
	sybilProtectionProvider module.Provider[*Engine, sybilprotection.SybilProtection],
	blockGadgetProvider module.Provider[*Engine, blockgadget.Gadget],
	slotGadgetProvider module.Provider[*Engine, slotgadget.Gadget],
	notarizationProvider module.Provider[*Engine, notarization.Notarization],
	attestationProvider module.Provider[*Engine, attestation.Attestations],
	ledgerProvider module.Provider[*Engine, ledger.Ledger],
	tipManagerProvider module.Provider[*Engine, tipmanager.TipManager],
	opts ...options.Option[Engine],
) (engine *Engine) {
	return options.Apply(
		&Engine{
			Events:        NewEvents(),
			Storage:       storageInstance,
			EvictionState: eviction.NewState(storageInstance.RootBlocks),
			Workers:       workers,
			errorHandler:  errorHandler,

			optsBootstrappedThreshold: 10 * time.Second,
			optsSnapshotDepth:         5,
		}, opts, func(e *Engine) {
			e.BlockCache = blocks.New(e.EvictionState, func() *iotago.SlotTimeProvider { return e.API().SlotTimeProvider() })

			e.BlockRequester = eventticker.New(e.optsBlockRequester...)

			e.SybilProtection = sybilProtectionProvider(e)
			e.BlockDAG = blockDAGProvider(e)
			e.Filter = filterProvider(e)
			e.Booker = bookerProvider(e)
			e.Clock = clockProvider(e)
			e.BlockGadget = blockGadgetProvider(e)
			e.SlotGadget = slotGadgetProvider(e)
			e.Notarization = notarizationProvider(e)
			e.Attestations = attestationProvider(e)
			e.Ledger = ledgerProvider(e)
			e.TipManager = tipManagerProvider(e)

			e.HookInitialized(lo.Batch(
				e.Storage.Settings().TriggerInitialized,
				e.Storage.Commitments().TriggerInitialized,
			))

			e.Storage.Settings().HookInitialized(func() {
				e.EvictionState.Initialize(e.Storage.Settings().LatestCommitment().Index())
			})
		},
		(*Engine).setupBlockStorage,
		(*Engine).setupEvictionState,
		(*Engine).setupBlockRequester,
		(*Engine).setupPruning,
		(*Engine).TriggerConstructed,
	)
}

func (e *Engine) Shutdown() {
	if !e.WasStopped() {
		e.TriggerStopped()

		e.BlockRequester.Shutdown()
		e.Attestations.Shutdown()
		e.Notarization.Shutdown()
		e.Booker.Shutdown()
		e.Ledger.Shutdown()
		e.BlockDAG.Shutdown()
		e.BlockGadget.Shutdown()
		e.SlotGadget.Shutdown()
		e.Clock.Shutdown()
		e.SybilProtection.Shutdown()
		e.Filter.Shutdown()
		e.Storage.Shutdown()
		e.Workers.Shutdown()
	}
}

func (e *Engine) ProcessBlockFromPeer(block *model.Block, source network.PeerID) {
	e.Filter.ProcessReceivedBlock(block, source)
	e.Events.BlockProcessed.Trigger(block.ID())
}

func (e *Engine) BlockFromCache(id iotago.BlockID) (*blocks.Block, bool) {
	return e.BlockCache.Block(id)
}

func (e *Engine) Block(id iotago.BlockID) (*model.Block, bool) {
	cachedBlock, exists := e.BlockCache.Block(id)
	if exists && !cachedBlock.IsRootBlock() {
		return cachedBlock.ModelBlock(), !cachedBlock.IsMissing()
	}

	// The block should've been in the block cache, so there's no need to check the storage.
	if !exists && id.Index() > e.Storage.Settings().LatestCommitment().Index() {
		return nil, false
	}

	s := e.Storage.Blocks(id.Index())
	if s == nil {
		return nil, false
	}
	modelBlock, err := s.Load(id)
	if err != nil {
		// TODO: log error?
		return nil, false
	}

	return modelBlock, modelBlock != nil
}

func (e *Engine) IsBootstrapped() (isBootstrapped bool) {
	e.isBootstrappedMutex.Lock()
	defer e.isBootstrappedMutex.Unlock()

	if e.isBootstrapped {
		return true
	}

	fmt.Println("IsBootstrapped", time.Since(e.Clock.Accepted().RelativeTime()), e.optsBootstrappedThreshold, e.Clock.Accepted().RelativeTime())
	if isBootstrapped = time.Since(e.Clock.Accepted().RelativeTime()) < e.optsBootstrappedThreshold && e.Notarization.IsBootstrapped(); isBootstrapped {
		e.isBootstrapped = true
	}

	return isBootstrapped
}

func (e *Engine) IsSynced() (isBootstrapped bool) {
	return e.IsBootstrapped() // && time.Since(e.Clock.PreAccepted().Time()) < e.optsBootstrappedThreshold
}

func (e *Engine) API() iotago.API {
	return e.Storage.Settings().API()
}

func (e *Engine) Initialize(snapshot ...string) (err error) {
	if !e.Storage.Settings().SnapshotImported() {
		if len(snapshot) == 0 || snapshot[0] == "" {
			panic("no snapshot path specified")
		}
		if err = e.readSnapshot(snapshot[0]); err != nil {
			return errors.Wrapf(err, "failed to read snapshot from file '%s'", snapshot)
		}

		// Only mark any pruning indexes if we loaded a non-genesis snapshot
		if e.Storage.Settings().LatestFinalizedSlot() > 0 {
			e.Storage.Prunable.PruneUntilSlot(e.Storage.Settings().LatestFinalizedSlot())
		}
	} else {
		e.Storage.Settings().UpdateAPI()
		e.Storage.Settings().TriggerInitialized()
		e.Storage.Commitments().TriggerInitialized()
		e.Storage.Prunable.RestoreFromDisk()
		e.EvictionState.PopulateFromStorage(e.Storage.Settings().LatestCommitment().Index())
		if err := e.Attestations.RestoreFromDisk(); err != nil {
			return errors.Wrap(err, "failed to restore attestations from disk")
		}
	}

	e.TriggerInitialized()

	fmt.Println("Engine Settings", e.Storage.Settings().String())

	return
}

func (e *Engine) WriteSnapshot(filePath string, targetSlot ...iotago.SlotIndex) (err error) {
	if len(targetSlot) == 0 {
		targetSlot = append(targetSlot, e.Storage.Settings().LatestCommitment().Index())
	}

	if fileHandle, err := os.Create(filePath); err != nil {
		return errors.Wrap(err, "failed to create snapshot file")
	} else if err = e.Export(fileHandle, targetSlot[0]); err != nil {
		return errors.Wrap(err, "failed to write snapshot")
	} else if err = fileHandle.Close(); err != nil {
		return errors.Wrap(err, "failed to close snapshot file")
	}

	return
}

func (e *Engine) Import(reader io.ReadSeeker) (err error) {
	if err = e.Storage.Settings().Import(reader); err != nil {
		return errors.Wrap(err, "failed to import settings")
	} else if err = e.Storage.Commitments().Import(reader); err != nil {
		return errors.Wrap(err, "failed to import commitments")
	} else if err = e.Ledger.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import ledger")
	} else if err = e.EvictionState.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import eviction state")
	} else if err = e.Attestations.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import attestation state")
	}

	return
}

func (e *Engine) Export(writer io.WriteSeeker, targetSlot iotago.SlotIndex) (err error) {
	targetCommitment, err := e.Storage.Commitments().Load(targetSlot)
	if err != nil {
		return errors.Wrapf(err, "failed to load target commitment at slot %d", targetSlot)
	}

	if err = e.Storage.Settings().Export(writer, targetCommitment.Commitment()); err != nil {
		return errors.Wrap(err, "failed to export settings")
	} else if err = e.Storage.Commitments().Export(writer, targetSlot); err != nil {
		return errors.Wrap(err, "failed to export commitments")
	} else if err = e.Ledger.Export(writer, targetSlot); err != nil {
		return errors.Wrap(err, "failed to export ledger")
	} else if err = e.EvictionState.Export(writer, e.Storage.Settings().LatestFinalizedSlot(), targetSlot); err != nil {
		// The rootcommitment is determined from the rootblocks. Therefore, we need to export starting from the last finalized slot.
		return errors.Wrap(err, "failed to export eviction state")
	} else if err = e.Attestations.Export(writer, targetSlot); err != nil {
		return errors.Wrap(err, "failed to export attestation state")
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

func (e *Engine) setupBlockStorage() {
	wp := e.Workers.CreatePool("BlockStorage", 1) // Using just 1 worker to avoid contention

	e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		store := e.Storage.Blocks(block.ID().Index())
		if store == nil {
			e.errorHandler(errors.Errorf("failed to store block with %s, storage with given index does not exist", block.ID()))
		}

		if err := store.Store(block.ModelBlock()); err != nil {
			e.errorHandler(errors.Wrapf(err, "failed to store block with %s", block.ID()))
		}
	}, event.WithWorkerPool(wp))
}

func (e *Engine) setupEvictionState() {
	e.Events.EvictionState.LinkTo(e.EvictionState.Events)

	wp := e.Workers.CreatePool("EvictionState", 1) // Using just 1 worker to avoid contention

	e.Events.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		block.ForEachParent(func(parent model.Parent) {
			// TODO: ONLY ADD STRONG PARENTS AFTER NOT DOWNLOADING PAST WEAK ARROWS
			// TODO: is this correct? could this lock acceptance in some extreme corner case? something like this happened, that confirmation is correctly advancing per block, but acceptance does not. I think it might have something to do with root blocks
			if parent.ID.Index() < block.ID().Index() && !e.EvictionState.IsRootBlock(parent.ID) {
				parentBlock, exists := e.Block(parent.ID)
				if !exists {
					e.errorHandler(errors.Errorf("cannot store root block (%s) because it is missing", parent.ID))
					return
				}
				e.EvictionState.AddRootBlock(parentBlock.ID(), parentBlock.Block().SlotCommitment.MustID())
			}
		})
	}, event.WithWorkerPool(wp))

	e.Events.Notarization.SlotCommitted.Hook(func(details *notarization.SlotCommittedDetails) {
		e.EvictionState.AdvanceActiveWindowToIndex(details.Commitment.Index())
	}, event.WithWorkerPool(wp))

	e.Events.EvictionState.SlotEvicted.Hook(e.BlockCache.EvictUntil)
}

func (e *Engine) setupBlockRequester() {
	e.Events.BlockRequester.LinkTo(e.BlockRequester.Events)

	e.Events.EvictionState.SlotEvicted.Hook(e.BlockRequester.EvictUntil)

	// We need to hook to make sure that the request is created before the block arrives to avoid a race condition
	// where we try to delete the request again before it is created. Thus, continuing to request forever.
	e.Events.BlockDAG.BlockMissing.Hook(func(block *blocks.Block) {
		// TODO: ONLY START REQUESTING WHEN NOT IN WARPSYNC RANGE (or just not attach outside)?
		e.BlockRequester.StartTicker(block.ID())
	})
	e.Events.BlockDAG.MissingBlockAttached.Hook(func(block *blocks.Block) {
		e.BlockRequester.StopTicker(block.ID())
	}, event.WithWorkerPool(e.Workers.CreatePool("BlockRequester", 1))) // Using just 1 worker to avoid contention
}

func (e *Engine) setupPruning() {
	e.Events.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		e.Storage.PruneUntilSlot(index)
	}, event.WithWorkerPool(e.Workers.CreatePool("PruneEngine", 1)))
}

func (e *Engine) readSnapshot(filePath string) (err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return errors.Wrap(err, "failed to open snapshot file")
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			panic(closeErr)
		}
	}()

	if err = e.Import(file); err != nil {
		return errors.Wrap(err, "failed to import snapshot")
	} else if err = e.Storage.Settings().SetSnapshotImported(true); err != nil {
		return errors.Wrap(err, "failed to set snapshot imported flag")
	}

	return
}

// EarliestRootCommitment is used to make sure that the chainManager knows the earliest possible
// commitment that blocks we are solidifying will refer to. Failing to do so will prevent those blocks
// from being processed as their chain will be deemed unsolid.
// lastFinalizedSlot is needed to make sure that the root commitment is not younger than the last finalized slot.
// If setting the root commitment based on the last evicted slot this basically means we won't be able to solidify another
// chain beyond a window based on eviction, which in turn is based on acceptance. In case of a partition, this behavior is
// clearly not desired.
func (e *Engine) EarliestRootCommitment(lastFinalizedSlot iotago.SlotIndex) (earliestCommitment *model.Commitment, valid bool) {
	earliestRootCommitmentID, valid := e.EvictionState.EarliestRootCommitmentID(lastFinalizedSlot)
	if !valid {
		return nil, false
	}
	rootCommitment, err := e.Storage.Commitments().Load(earliestRootCommitmentID.Index())
	if err != nil {
		panic(fmt.Sprintln("could not load earliest commitment after engine initialization", err))
	}

	return rootCommitment, true
}

func (e *Engine) ErrorHandler(componentName string) func(error) {
	return func(err error) {
		e.errorHandler(errors.Wrap(err, componentName))
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Options //////////////////////////////////////////////////////////////////////////////////////////////////////

func WithBootstrapThreshold(threshold time.Duration) options.Option[Engine] {
	return func(e *Engine) {
		e.optsBootstrappedThreshold = threshold
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

func WithRequesterOptions(opts ...options.Option[eventticker.EventTicker[iotago.SlotIndex, iotago.BlockID]]) options.Option[Engine] {
	return func(e *Engine) {
		e.optsBlockRequester = append(e.optsBlockRequester, opts...)
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
