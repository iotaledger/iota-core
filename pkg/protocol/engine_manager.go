package protocol

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

type EngineManager struct {
	MainEngine reactive.Variable[*engine.Engine]

	protocol  *Protocol
	worker    *workerpool.WorkerPool
	directory *utils.Directory

	*module.ReactiveModule
}

func NewEngineManager(protocol *Protocol) *EngineManager {
	e := &EngineManager{
		MainEngine:     reactive.NewVariable[*engine.Engine](),
		ReactiveModule: protocol.NewReactiveSubModule("Engines"),
		protocol:       protocol,
		worker:         protocol.Workers.CreatePool("Engines", workerpool.WithWorkerCount(1)),
		directory:      utils.NewDirectory(protocol.Options.BaseDirectory),
	}

	protocol.Constructed.OnTrigger(func() {
		unsubscribe := lo.Batch(
			e.syncMainEngineFromMainChain(),
			e.syncMainEngineInfoFile(),
			e.injectEngineInstances(),
		)

		e.Shutdown.OnTrigger(func() {
			unsubscribe()

			e.worker.Shutdown().ShutdownComplete.Wait()

			e.Stopped.Trigger()
		})

		e.Initialized.Trigger()
	})

	e.Constructed.Trigger()

	return e
}

func (e *EngineManager) LoadMainEngine(snapshotPath string) (*engine.Engine, error) {
	info := &engineInfo{}
	if err := ioutils.ReadJSONFromFile(e.infoFilePath(), info); err != nil && !ierrors.Is(err, os.ErrNotExist) {
		return nil, ierrors.Errorf("unable to read engine info file: %w", err)
	}

	// load previous engine as main engine if it exists.
	if len(info.Name) > 0 {
		if exists, isDirectory, err := ioutils.PathExists(e.directory.Path(info.Name)); err == nil && exists && isDirectory {
			e.MainEngine.Set(e.loadEngineInstanceFromSnapshot(info.Name, snapshotPath))
		}
	}

	// load new engine if no previous engine exists.
	e.MainEngine.Compute(func(mainEngine *engine.Engine) *engine.Engine {
		if mainEngine != nil {
			return mainEngine
		}

		return e.loadEngineInstanceFromSnapshot(lo.PanicOnErr(uuid.NewUUID()).String(), snapshotPath)
	})

	// cleanup candidates
	if err := e.CleanupCandidates(); err != nil {
		return nil, err
	}

	return e.MainEngine.Get(), nil
}

func (e *EngineManager) ForkAtSlot(slot iotago.SlotIndex) (*engine.Engine, error) {
	newEngineAlias := lo.PanicOnErr(uuid.NewUUID()).String()
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", newEngineAlias[0:8])
	}

	// copy raw data on disk.
	newStorage, err := storage.Clone(e.MainEngine.Get().Storage, e.directory.Path(newEngineAlias), DatabaseVersion, errorHandler, e.protocol.Options.StorageOptions...)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy storage from active engine instance (%s) to new engine instance (%s)", e.MainEngine.Get().Storage.Directory(), e.directory.Path(newEngineAlias))
	}

	// remove commitments that after forking point.
	latestCommitment := newStorage.Settings().LatestCommitment()
	if err := newStorage.Commitments().Rollback(slot, latestCommitment.Slot()); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback commitments")
	}
	// create temporary components and rollback their permanent state, which will be reflected on disk.
	evictionState := eviction.NewState(newStorage.LatestNonEmptySlot(), newStorage.RootBlocks)
	evictionState.Initialize(latestCommitment.Slot())

	blockCache := blocks.New(evictionState, newStorage.Settings().APIProvider())
	accountsManager := accountsledger.New(newStorage.Settings().APIProvider(), blockCache.Block, newStorage.AccountDiffs, newStorage.Accounts())

	accountsManager.SetLatestCommittedSlot(latestCommitment.Slot())
	if err := accountsManager.Rollback(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback accounts manager")
	}

	if err := evictionState.Rollback(newStorage.Settings().LatestFinalizedSlot(), slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback eviction state")
	}
	if err := newStorage.Ledger().Rollback(slot); err != nil {
		return nil, err
	}

	targetCommitment, err := newStorage.Commitments().Load(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving commitment for target index %d", slot)
	}

	if err := newStorage.Settings().Rollback(targetCommitment); err != nil {
		return nil, err
	}

	if err := newStorage.RollbackPrunable(slot); err != nil {
		return nil, err
	}

	candidateEngine := e.loadEngineInstanceWithStorage(newEngineAlias, newStorage)

	// rollback attestations already on created engine instance, because this action modifies the in-memory storage.
	if err := candidateEngine.Attestations.Rollback(slot); err != nil {
		return nil, ierrors.Wrap(err, "error while rolling back attestations storage on candidate engine")
	}

	return candidateEngine, nil
}

func (e *EngineManager) CleanupCandidates() error {
	activeDir := filepath.Base(e.MainEngine.Get().Storage.Directory())

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

func (e *EngineManager) loadEngineInstanceFromSnapshot(engineAlias string, snapshotPath string) *engine.Engine {
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	e.protocol.Options.EngineOptions = append(e.protocol.Options.EngineOptions, engine.WithSnapshotPath(snapshotPath))

	return e.loadEngineInstanceWithStorage(engineAlias, storage.Create(e.directory.Path(engineAlias), DatabaseVersion, errorHandler, e.protocol.Options.StorageOptions...))
}

func (e *EngineManager) loadEngineInstanceWithStorage(engineAlias string, storage *storage.Storage) *engine.Engine {
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	return engine.New(e.protocol.Workers.CreateGroup(engineAlias), errorHandler, storage, e.protocol.Options.FilterProvider, e.protocol.Options.CommitmentFilterProvider, e.protocol.Options.BlockDAGProvider, e.protocol.Options.BookerProvider, e.protocol.Options.ClockProvider, e.protocol.Options.BlockGadgetProvider, e.protocol.Options.SlotGadgetProvider, e.protocol.Options.SybilProtectionProvider, e.protocol.Options.NotarizationProvider, e.protocol.Options.AttestationProvider, e.protocol.Options.LedgerProvider, e.protocol.Options.SchedulerProvider, e.protocol.Options.TipManagerProvider, e.protocol.Options.TipSelectionProvider, e.protocol.Options.RetainerProvider, e.protocol.Options.UpgradeOrchestratorProvider, e.protocol.Options.SyncManagerProvider, e.protocol.Options.EngineOptions...)
}

func (e *EngineManager) syncMainEngineFromMainChain() (unsubscribe func()) {
	return e.protocol.MainChain.OnUpdateWithContext(func(_, mainChain *Chain, unsubscribeOnUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		unsubscribeOnUpdate(func() func() {
			return e.MainEngine.InheritFrom(mainChain.SpawnedEngine)
		})
	})
}

func (e *EngineManager) syncMainEngineInfoFile() (unsubscribe func()) {
	return e.MainEngine.OnUpdate(func(_, mainEngine *engine.Engine) {
		if err := ioutils.WriteJSONToFile(e.infoFilePath(), &engineInfo{Name: filepath.Base(mainEngine.Storage.Directory())}, 0o644); err != nil {
			e.LogError("unable to write engine info file", "err", err)
		}
	})
}

func (e *EngineManager) injectEngineInstances() (unsubscribe func()) {
	return e.protocol.OnChainCreated(func(chain *Chain) {
		chain.VerifyState.OnUpdate(func(_, instantiate bool) {
			e.worker.Submit(func() {
				if !instantiate {
					chain.SpawnedEngine.Set(nil)

					return
				}

				if newEngine, err := func() (*engine.Engine, error) {
					if e.MainEngine.Get() == nil {
						return e.LoadMainEngine(e.protocol.Options.SnapshotPath)
					}

					return e.ForkAtSlot(chain.ForkingPoint.Get().Slot() - 1)
				}(); err != nil {
					e.LogError("failed to create new engine instance", "err", err)
				} else {
					e.protocol.Network.OnShutdown(newEngine.Shutdown)

					chain.SpawnedEngine.Set(newEngine)
				}
			})
		})
	})
}

const engineInfoFile = "info"

type engineInfo struct {
	Name string `json:"name"`
}
