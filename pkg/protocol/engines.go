package protocol

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	"github.com/iotaledger/iota-core/pkg/storage"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Engines is a subcomponent of the protocol that exposes the engines that are managed by the protocol.
type Engines struct {
	// Main contains the main engine.
	Main reactive.Variable[*engine.Engine]

	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// worker contains the worker pool that is used to process changes to the engine instances asynchronously.
	worker *workerpool.WorkerPool

	// directory contains the directory that is used to store the engine instances on disk.
	directory *utils.Directory

	// ReactiveModule embeds a reactive module that provides default API for logging and lifecycle management.
	module.Module
}

// newEngines creates a new Engines instance.
func newEngines(protocol *Protocol) *Engines {
	e := &Engines{
		Main:      reactive.NewVariable[*engine.Engine](),
		Module:    protocol.NewSubModule("Engines"),
		protocol:  protocol,
		worker:    protocol.Workers.CreatePool("Engines", workerpool.WithWorkerCount(1)),
		directory: utils.NewDirectory(protocol.Options.BaseDirectory),
	}

	protocol.ConstructedEvent().OnTrigger(func() {
		shutdown := lo.BatchReverse(
			e.initLogging(),

			e.syncMainEngineFromMainChain(),
			e.syncMainEngineInfoFile(),
			e.injectEngineInstances(),
		)

		e.ShutdownEvent().OnTrigger(func() {
			shutdown()

			e.StoppedEvent().Trigger()
		})

		e.InitializedEvent().Trigger()
	})

	e.ConstructedEvent().Trigger()

	return e
}

// initLogging initializes the logging for this component.
func (e *Engines) initLogging() (shutdown func()) {
	return e.Main.LogUpdates(e, log.LevelTrace, "Main", (*engine.Engine).LogName)
}

// ForkAtSlot creates a new engine instance that forks from the main engine at the given slot.
func (e *Engines) ForkAtSlot(slot iotago.SlotIndex) (*engine.Engine, error) {
	newEngineAlias := lo.PanicOnErr(uuid.NewUUID()).String()
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", newEngineAlias[0:8])
	}

	// copy raw data on disk.
	newStorage, err := storage.Clone(e, e.Main.Get().Storage, e.directory.Path(newEngineAlias), DatabaseVersion, errorHandler, e.protocol.Options.StorageOptions...)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy storage from active engine instance (%s) to new engine instance (%s)", e.Main.Get().Storage.Directory(), e.directory.Path(newEngineAlias))
	}

	// remove commitments that after forking point.
	latestCommitment := newStorage.Settings().LatestCommitment()
	if err = newStorage.Commitments().Rollback(slot, latestCommitment.Slot()); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback commitments")
	}

	// some components are automatically rolled back by deleting their data on disk (e.g. slot based storage).
	// some other components need to be rolled back manually, like the UTXO ledger for example.
	// we need to create temporary components to rollback their permanent state, which will be reflected on disk.
	evictionState := eviction.NewState(newStorage.Settings(), newStorage.RootBlocks)
	evictionState.Initialize(latestCommitment.Slot())

	blockCache := blocks.New(evictionState, newStorage.Settings().APIProvider())
	accountsManager := accountsledger.New(module.New(log.NewLogger(log.WithName("ForkedAccountsLedger"))), newStorage.Settings().APIProvider(), blockCache.Block, newStorage.AccountDiffs, newStorage.Accounts())

	accountsManager.SetLatestCommittedSlot(latestCommitment.Slot())
	if err = accountsManager.Rollback(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback accounts manager")
	}

	if err = evictionState.Rollback(newStorage.Settings().LatestFinalizedSlot(), slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to rollback eviction state")
	}
	if err = newStorage.Ledger().Rollback(slot); err != nil {
		return nil, err
	}

	targetCommitment, err := newStorage.Commitments().Load(slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving commitment for target index %d", slot)
	}

	if err = newStorage.Settings().Rollback(targetCommitment); err != nil {
		return nil, err
	}

	if err = newStorage.Rollback(slot); err != nil {
		return nil, err
	}

	candidateEngine := e.loadEngineInstanceWithStorage(newEngineAlias, newStorage)

	// rollback attestations already on created engine instance, because this action modifies the in-memory storage.
	if err = candidateEngine.Attestations.Rollback(slot); err != nil {
		return nil, ierrors.Wrap(err, "error while rolling back attestations storage on candidate engine")
	}

	return candidateEngine, nil
}

// loadMainEngine loads the main engine from disk or creates a new one if no engine exists.
func (e *Engines) loadMainEngine(snapshotPath string, commitmentCheck bool) (*engine.Engine, error) {
	info := &engineInfo{}
	if err := ioutils.ReadJSONFromFile(e.infoFilePath(), info); err != nil && !ierrors.Is(err, os.ErrNotExist) {
		return nil, ierrors.Wrap(err, "unable to read engine info file")
	}

	//nolint:revive
	e.Main.Compute(func(mainEngine *engine.Engine) *engine.Engine {
		// load previous engine as main engine if it exists.
		if len(info.Name) > 0 {
			if exists, isDirectory, err := ioutils.PathExists(e.directory.Path(info.Name)); err == nil && exists && isDirectory {
				return e.loadEngineInstanceFromSnapshot(info.Name, snapshotPath, commitmentCheck)
			}
		}

		// load new engine if no previous engine exists.
		return e.loadEngineInstanceFromSnapshot(lo.PanicOnErr(uuid.NewUUID()).String(), snapshotPath, commitmentCheck)
	})

	// cleanup candidates
	if err := e.cleanupCandidates(); err != nil {
		return nil, err
	}

	return e.Main.Get(), nil
}

// cleanupCandidates removes all engine instances that are not the main engine.
func (e *Engines) cleanupCandidates() error {
	activeDir := filepath.Base(e.Main.Get().Storage.Directory())

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

// infoFilePath returns the path to the engine info file.
func (e *Engines) infoFilePath() string {
	return e.directory.Path(engineInfoFile)
}

// loadEngineInstanceFromSnapshot loads an engine instance from a snapshot.
func (e *Engines) loadEngineInstanceFromSnapshot(engineAlias string, snapshotPath string, commitmentCheck bool) *engine.Engine {
	errorHandler := func(err error) {
		e.protocol.LogError("engine error", "err", err, "name", engineAlias[0:8])
	}

	return e.loadEngineInstanceWithStorage(engineAlias, storage.Create(e, e.directory.Path(engineAlias), DatabaseVersion, errorHandler, e.protocol.Options.StorageOptions...), engine.WithSnapshotPath(snapshotPath), engine.WithCommitmentCheck(commitmentCheck))
}

// loadEngineInstanceWithStorage loads an engine instance with the given storage.
func (e *Engines) loadEngineInstanceWithStorage(engineAlias string, storage *storage.Storage, engineOptions ...options.Option[engine.Engine]) *engine.Engine {
	return engine.New(
		e.protocol,
		e.protocol.Workers.CreateGroup(engineAlias),
		storage,
		e.protocol.Options.PreSolidFilterProvider,
		e.protocol.Options.PostSolidFilterProvider,
		e.protocol.Options.BlockDAGProvider,
		e.protocol.Options.BookerProvider,
		e.protocol.Options.ClockProvider,
		e.protocol.Options.BlockGadgetProvider,
		e.protocol.Options.SlotGadgetProvider,
		e.protocol.Options.SybilProtectionProvider,
		e.protocol.Options.NotarizationProvider,
		e.protocol.Options.SyncManagerProvider,
		e.protocol.Options.AttestationProvider,
		e.protocol.Options.LedgerProvider,
		e.protocol.Options.SchedulerProvider,
		e.protocol.Options.TipManagerProvider,
		e.protocol.Options.TipSelectionProvider,
		e.protocol.Options.BlockRetainerProvider,
		e.protocol.Options.TransactionRetainerProvider,
		e.protocol.Options.UpgradeOrchestratorProvider,
		append(e.protocol.Options.EngineOptions, engineOptions...)...,
	)
}

// syncMainEngineFromMainChain syncs the main engine from the main chain.
func (e *Engines) syncMainEngineFromMainChain() (shutdown func()) {
	return e.protocol.Chains.Main.WithNonEmptyValue(func(mainChain *Chain) (shutdown func()) {
		return e.Main.DeriveValueFrom(reactive.NewDerivedVariable(func(currentMainEngine *engine.Engine, newMainEngine *engine.Engine) *engine.Engine {
			return lo.Cond(newMainEngine == nil, currentMainEngine, newMainEngine)
		}, mainChain.Engine, e.Main.Get()))
	})
}

// syncMainEngineInfoFile syncs the engine info file with the main engine.
func (e *Engines) syncMainEngineInfoFile() (shutdown func()) {
	return e.Main.OnUpdate(func(_ *engine.Engine, mainEngine *engine.Engine) {
		if mainEngine != nil {
			if err := ioutils.WriteJSONToFile(e.infoFilePath(), &engineInfo{Name: filepath.Base(mainEngine.Storage.Directory())}, 0o644); err != nil {
				e.LogError("unable to write engine info file", "err", err)
			}
		}
	})
}

// injectEngineInstances injects engine instances into the chains (when requested).
func (e *Engines) injectEngineInstances() (shutdown func()) {
	return e.protocol.Chains.WithElements(func(chain *Chain) (shutdown func()) {
		return chain.StartEngine.OnUpdate(func(_ bool, startEngine bool) {
			e.worker.Submit(func() {
				if !startEngine {
					chain.Engine.Set(nil)

					return
				}

				if newEngine, err := func() (*engine.Engine, error) {
					if e.Main.Get() == nil {
						return e.loadMainEngine(e.protocol.Options.SnapshotPath, e.protocol.Options.CommitmentCheck)
					}

					return e.ForkAtSlot(chain.ForkingPoint.Get().Slot() - 1)
				}(); err != nil {
					e.LogError("failed to create new engine instance", "err", err)
				} else {
					e.protocol.Network.OnShutdown(func() { newEngine.ShutdownEvent().Trigger() })

					chain.Engine.Set(newEngine)
				}
			})
		})
	})
}

// engineInfoFile is the name of the engine info file.
const engineInfoFile = "info"

// engineInfo is the structure of the engine info file.
type engineInfo struct {
	// Name contains the name of the engine.
	Name string `json:"name"`
}
