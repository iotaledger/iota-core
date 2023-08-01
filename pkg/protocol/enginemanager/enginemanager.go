package enginemanager

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/attestation"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/commitmentfilter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/scheduler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/blockgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/consensus/slotgadget"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
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

// region EngineManager ////////////////////////////////////////////////////////////////////////////////////////////////

type EngineManager struct {
	directory      *utils.Directory
	dbVersion      byte
	storageOptions []options.Option[storage.Storage]
	workers        *workerpool.Group
	errorHandler   func(error)

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

	activeInstance *engine.Engine
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
) *EngineManager {
	return &EngineManager{
		workers:                     workers,
		errorHandler:                errorHandler,
		directory:                   utils.NewDirectory(dir),
		dbVersion:                   dbVersion,
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
			e.activeInstance = e.loadEngineInstance(info.Name, snapshotPath)
		}
	}

	if e.activeInstance == nil {
		// Start with a new instance and set to active
		instance := e.newEngineInstance(snapshotPath)
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

func (e *EngineManager) loadEngineInstance(dirName string, snapshotPath string) *engine.Engine {
	errorHandler := func(err error) {
		e.errorHandler(ierrors.Wrapf(err, "engine (%s)", dirName[0:8]))
	}

	return engine.New(e.workers.CreateGroup(dirName),
		errorHandler,
		storage.New(e.directory.Path(dirName), e.dbVersion, errorHandler, e.storageOptions...),
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
		append(e.engineOptions, engine.WithSnapshotPath(snapshotPath))...,
	)
}

func (e *EngineManager) newEngineInstance(snapshotPath string) *engine.Engine {
	dirName := lo.PanicOnErr(uuid.NewUUID()).String()
	return e.loadEngineInstance(dirName, snapshotPath)
}

func (e *EngineManager) ForkEngineAtSlot(index iotago.SlotIndex) (*engine.Engine, error) {
	// Dump a snapshot at the target index
	snapshotPath := filepath.Join(os.TempDir(), fmt.Sprintf("snapshot_%d_%s.bin", index, lo.PanicOnErr(uuid.NewUUID())))
	if err := e.activeInstance.WriteSnapshot(snapshotPath, index); err != nil {
		return nil, ierrors.Wrapf(err, "error exporting snapshot for index %s", index)
	}

	return e.newEngineInstance(snapshotPath), nil
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
