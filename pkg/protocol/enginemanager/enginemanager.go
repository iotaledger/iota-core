package enginemanager

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
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
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const engineInfoFile = "info"

type engineInfo struct {
	Name string `json:"name"`
}

type EngineManager struct {
	directory      *utils.Directory
	dbVersion      byte
	workers        *workerpool.Group
	errorHandler   func(error)
	engineCreated  *event.Event1[*engine.Engine]
	activeInstance *engine.Engine

	storageOptions              []options.Option[storage.Storage]
	engineOptions               []options.Option[engine.Engine]
	filterProvider              module.Provider[*engine.Engine, filter.Filter]
	commitmentFilterProvider    module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter]
	blockDAGProvider            module.Provider[*engine.Engine, blockdag.BlockDAG]
	bookerProvider              module.Provider[*engine.Engine, booker.Booker]
	clockProvider               module.Provider[*engine.Engine, clock.Clock]
	blockGadgetProvider         module.Provider[*engine.Engine, blockgadget.Gadget]
	slotGadgetProvider          module.Provider[*engine.Engine, slotgadget.Gadget]
	sybilProtectionProvider     module.Provider[*engine.Engine, sybilprotection.SybilProtection]
	notarizationProvider        module.Provider[*engine.Engine, notarization.Notarization]
	attestationProvider         module.Provider[*engine.Engine, attestation.Attestations]
	ledgerProvider              module.Provider[*engine.Engine, ledger.Ledger]
	schedulerProvider           module.Provider[*engine.Engine, scheduler.Scheduler]
	tipManagerProvider          module.Provider[*engine.Engine, tipmanager.TipManager]
	tipSelectionProvider        module.Provider[*engine.Engine, tipselection.TipSelection]
	retainerProvider            module.Provider[*engine.Engine, retainer.Retainer]
	upgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator]
	syncManagerProvider         module.Provider[*engine.Engine, syncmanager.SyncManager]
}

func New(
	workers *workerpool.Group,
	errorHandler func(error),
	dir string,
	dbVersion byte,
	storageOptions []options.Option[storage.Storage],
	engineOptions []options.Option[engine.Engine],
	filterProvider module.Provider[*engine.Engine, filter.Filter],
	commitmentFilterProvider module.Provider[*engine.Engine, commitmentfilter.CommitmentFilter],
	blockDAGProvider module.Provider[*engine.Engine, blockdag.BlockDAG],
	bookerProvider module.Provider[*engine.Engine, booker.Booker],
	clockProvider module.Provider[*engine.Engine, clock.Clock],
	blockGadgetProvider module.Provider[*engine.Engine, blockgadget.Gadget],
	slotGadgetProvider module.Provider[*engine.Engine, slotgadget.Gadget],
	sybilProtectionProvider module.Provider[*engine.Engine, sybilprotection.SybilProtection],
	notarizationProvider module.Provider[*engine.Engine, notarization.Notarization],
	attestationProvider module.Provider[*engine.Engine, attestation.Attestations],
	ledgerProvider module.Provider[*engine.Engine, ledger.Ledger],
	schedulerProvider module.Provider[*engine.Engine, scheduler.Scheduler],
	tipManagerProvider module.Provider[*engine.Engine, tipmanager.TipManager],
	tipSelectionProvider module.Provider[*engine.Engine, tipselection.TipSelection],
	retainerProvider module.Provider[*engine.Engine, retainer.Retainer],
	upgradeOrchestratorProvider module.Provider[*engine.Engine, upgrade.Orchestrator],
	syncManagerProvider module.Provider[*engine.Engine, syncmanager.SyncManager],
) *EngineManager {
	return &EngineManager{
		directory:                   utils.NewDirectory(dir),
		dbVersion:                   dbVersion,
		workers:                     workers,
		errorHandler:                errorHandler,
		engineCreated:               event.New1[*engine.Engine](),
		storageOptions:              storageOptions,
		engineOptions:               engineOptions,
		filterProvider:              filterProvider,
		commitmentFilterProvider:    commitmentFilterProvider,
		blockDAGProvider:            blockDAGProvider,
		bookerProvider:              bookerProvider,
		clockProvider:               clockProvider,
		blockGadgetProvider:         blockGadgetProvider,
		slotGadgetProvider:          slotGadgetProvider,
		sybilProtectionProvider:     sybilProtectionProvider,
		notarizationProvider:        notarizationProvider,
		attestationProvider:         attestationProvider,
		ledgerProvider:              ledgerProvider,
		schedulerProvider:           schedulerProvider,
		tipManagerProvider:          tipManagerProvider,
		tipSelectionProvider:        tipSelectionProvider,
		retainerProvider:            retainerProvider,
		upgradeOrchestratorProvider: upgradeOrchestratorProvider,
		syncManagerProvider:         syncManagerProvider,
	}
}

func (e *EngineManager) LoadActiveEngine(snapshotPath string) (*engine.Engine, error) {
	info := &engineInfo{}
	if err := ioutils.ReadJSONFromFile(e.infoFilePath(), info); err != nil {
		if !ierrors.Is(err, os.ErrNotExist) {
			return nil, ierrors.Errorf("unable to read engine info file: %w", err)
		}
	}

	if len(info.Name) > 0 {
		if exists, isDirectory, err := ioutils.PathExists(e.directory.Path(info.Name)); err == nil && exists && isDirectory {
			// Load previous engine as active
			e.activeInstance = e.loadEngineInstanceFromSnapshot(info.Name, snapshotPath)
		}
	}

	if e.activeInstance == nil {
		// Start with a new instance and set to active
		instance := e.loadEngineInstanceFromSnapshot(lo.PanicOnErr(uuid.NewUUID()).String(), snapshotPath)

		if err := e.SetActiveInstance(instance); err != nil {
			return nil, err
		}
	}

	// Cleanup non-active instances
	if err := e.CleanupNonActive(); err != nil {
		return nil, err
	}

	return e.activeInstance, nil
}

func (e *EngineManager) CleanupNonActive() error {
	activeDir := filepath.Base(e.activeInstance.Storage.Directory())

	dirs, err := e.directory.SubDirs()
	if err != nil {
		return ierrors.Wrapf(err, "unable to list subdirectories of %s", e.directory.Path())
	}
	for _, dir := range dirs {
		if dir == activeDir {
			continue
		}
		if err := e.directory.RemoveSubdir(dir); err != nil {
			return ierrors.Wrapf(err, "unable to remove subdirectory %s", dir)
		}
	}

	return nil
}

func (e *EngineManager) infoFilePath() string {
	return e.directory.Path(engineInfoFile)
}

func (e *EngineManager) SetActiveInstance(instance *engine.Engine) error {
	e.activeInstance = instance

	info := &engineInfo{
		Name: filepath.Base(instance.Storage.Directory()),
	}

	return ioutils.WriteJSONToFile(e.infoFilePath(), info, 0o644)
}

func (e *EngineManager) loadEngineInstanceFromSnapshot(engineAlias string, snapshotPath string) *engine.Engine {
	errorHandler := func(err error) {
		e.errorHandler(ierrors.Wrapf(err, "engine (%s)", engineAlias[0:8]))
	}

	e.engineOptions = append(e.engineOptions, engine.WithSnapshotPath(snapshotPath))

	return e.loadEngineInstanceWithStorage(engineAlias, storage.Create(e.directory.Path(engineAlias), e.dbVersion, errorHandler, e.storageOptions...))
}

func (e *EngineManager) loadEngineInstanceWithStorage(engineAlias string, storage *storage.Storage) *engine.Engine {
	errorHandler := func(err error) {
		e.errorHandler(ierrors.Wrapf(err, "engine (%s)", engineAlias[0:8]))
	}

	newEngine := engine.New(e.workers.CreateGroup(engineAlias),
		errorHandler,
		storage,
		e.filterProvider,
		e.commitmentFilterProvider,
		e.blockDAGProvider,
		e.bookerProvider,
		e.clockProvider,
		e.blockGadgetProvider,
		e.slotGadgetProvider,
		e.sybilProtectionProvider,
		e.notarizationProvider,
		e.attestationProvider,
		e.ledgerProvider,
		e.schedulerProvider,
		e.tipManagerProvider,
		e.tipSelectionProvider,
		e.retainerProvider,
		e.upgradeOrchestratorProvider,
		e.syncManagerProvider,
		e.engineOptions...,
	)

	e.engineCreated.Trigger(newEngine)

	return newEngine
}

func (e *EngineManager) ForkEngineAtSlot(index iotago.SlotIndex) (*engine.Engine, error) {
	engineAlias := newEngineAlias()
	errorHandler := func(err error) {
		e.errorHandler(ierrors.Wrapf(err, "engine (%s)", engineAlias[0:8]))
	}

	// Copy raw data on disk.
	newStorage, err := storage.Clone(e.activeInstance.Storage, e.directory.Path(engineAlias), e.dbVersion, errorHandler, e.storageOptions...)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy storage from active engine instance (%s) to new engine instance (%s)", e.activeInstance.Storage.Directory(), e.directory.Path(engineAlias))
	}

	// Remove commitments that after forking point.
	latestCommitment := newStorage.Settings().LatestCommitment()
	if err := newStorage.Commitments().Rollback(index, latestCommitment.Index()); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback commitments")
	}
	// Create temporary components and rollback their permanent state, which will be reflected on disk.
	evictionState := eviction.NewState(newStorage.LatestNonEmptySlot(), newStorage.RootBlocks)
	evictionState.Initialize(latestCommitment.Index())

	blockCache := blocks.New(evictionState, newStorage.Settings().APIProvider())
	accountsManager := accountsledger.New(newStorage.Settings().APIProvider(), blockCache.Block, newStorage.AccountDiffs, newStorage.Accounts())

	accountsManager.SetLatestCommittedSlot(latestCommitment.Index())
	if err := accountsManager.Rollback(index); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback accounts manager")
	}

	if err := evictionState.Rollback(newStorage.Settings().LatestFinalizedSlot(), index); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback eviction state")
	}
	if err := newStorage.Ledger().Rollback(index); err != nil {
		return nil, err
	}

	targetCommitment, err := newStorage.Commitments().Load(index)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving commitment for target index %d", index)
	}

	if err := newStorage.Settings().Rollback(targetCommitment); err != nil {
		return nil, err
	}

	if err := newStorage.RollbackPrunable(index); err != nil {
		return nil, err
	}

	candidateEngine := e.loadEngineInstanceWithStorage(engineAlias, newStorage)

	// Rollback attestations already on created engine instance, because this action modifies the in-memory storage.
	if err := candidateEngine.Attestations.Rollback(index); err != nil {
		return nil, ierrors.Wrap(err, "error while rolling back attestations storage on candidate engine")
	}

	return candidateEngine, nil
}

func (e *EngineManager) OnEngineCreated(handler func(*engine.Engine)) (unsubscribe func()) {
	return e.engineCreated.Hook(handler).Unhook
}

func newEngineAlias() string {
	return lo.PanicOnErr(uuid.NewUUID()).String()
}
