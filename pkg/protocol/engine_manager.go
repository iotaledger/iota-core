package protocol

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

const engineInfoFile = "info"

type engineInfo struct {
	Name string `json:"name"`
}

type EngineManager struct {
	protocol       *Protocol
	directory      *utils.Directory
	activeInstance *engine.Engine
}

func NewEngineManager(protocol *Protocol) *EngineManager {
	e := &EngineManager{
		protocol:  protocol,
		directory: utils.NewDirectory(protocol.options.BaseDirectory),
	}

	protocol.HookConstructed(func() {
		protocol.OnChainCreated(e.provideEngineIfRequested)
	})

	return e
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
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	e.protocol.options.EngineOptions = append(e.protocol.options.EngineOptions, engine.WithSnapshotPath(snapshotPath))

	return e.loadEngineInstanceWithStorage(engineAlias, storage.Create(e.directory.Path(engineAlias), DatabaseVersion, errorHandler, e.protocol.options.StorageOptions...))
}

func (e *EngineManager) loadEngineInstanceWithStorage(engineAlias string, storage *storage.Storage) *engine.Engine {
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	return engine.New(e.protocol.Workers.CreateGroup(engineAlias), errorHandler, storage, e.protocol.options.FilterProvider, e.protocol.options.CommitmentFilterProvider, e.protocol.options.BlockDAGProvider, e.protocol.options.BookerProvider, e.protocol.options.ClockProvider, e.protocol.options.BlockGadgetProvider, e.protocol.options.SlotGadgetProvider, e.protocol.options.SybilProtectionProvider, e.protocol.options.NotarizationProvider, e.protocol.options.AttestationProvider, e.protocol.options.LedgerProvider, e.protocol.options.SchedulerProvider, e.protocol.options.TipManagerProvider, e.protocol.options.TipSelectionProvider, e.protocol.options.RetainerProvider, e.protocol.options.UpgradeOrchestratorProvider, e.protocol.options.SyncManagerProvider, e.protocol.options.EngineOptions...)
}

func (e *EngineManager) ForkEngineAtSlot(index iotago.SlotIndex) (*engine.Engine, error) {
	engineAlias := newEngineAlias()
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	// Copy raw data on disk.
	newStorage, err := storage.Clone(e.activeInstance.Storage, e.directory.Path(engineAlias), DatabaseVersion, errorHandler, e.protocol.options.StorageOptions...)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy storage from active engine instance (%s) to new engine instance (%s)", e.activeInstance.Storage.Directory(), e.directory.Path(engineAlias))
	}

	// Remove commitments that after forking point.
	latestCommitment := newStorage.Settings().LatestCommitment()
	if err := newStorage.Commitments().Rollback(index, latestCommitment.Slot()); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback commitments")
	}
	// Create temporary components and rollback their permanent state, which will be reflected on disk.
	evictionState := eviction.NewState(newStorage.LatestNonEmptySlot(), newStorage.RootBlocks)
	evictionState.Initialize(latestCommitment.Slot())

	blockCache := blocks.New(evictionState, newStorage.Settings().APIProvider())
	accountsManager := accountsledger.New(newStorage.Settings().APIProvider(), blockCache.Block, newStorage.AccountDiffs, newStorage.Accounts())

	accountsManager.SetLatestCommittedSlot(latestCommitment.Slot())
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

func (e *EngineManager) provideEngineIfRequested(chain *Chain) {
	chain.SpawnEngine.OnUpdate(func(_, instantiate bool) {
		// TODO: MOVE TO WORKERPOOL
		go func() {
			if !instantiate {
				chain.SpawnedEngine.Set(nil)

				return
			}

			if currentEngine := chain.Engine.Get(); currentEngine == nil {
				mainEngine, err := e.LoadActiveEngine(e.protocol.options.SnapshotPath)
				if err != nil {
					panic(fmt.Sprintf("could not load active engine: %s", err))
				}

				chain.SpawnedEngine.Set(mainEngine)

				e.protocol.NetworkManager.OnShutdown(mainEngine.Shutdown)
			} else {
				forkingPoint := chain.ForkingPoint.Get()
				snapshotTargetSlot := forkingPoint.Slot() - 1

				candidateEngineInstance, err := e.ForkEngineAtSlot(snapshotTargetSlot)
				if err != nil {
					panic(ierrors.Wrap(err, "error creating new candidate engine"))
				}

				chain.SpawnedEngine.Set(candidateEngineInstance)

				e.protocol.NetworkManager.OnShutdown(candidateEngineInstance.Shutdown)
			}
		}()
	})
}

func newEngineAlias() string {
	return lo.PanicOnErr(uuid.NewUUID()).String()
}
