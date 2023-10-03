package protocol

import (
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// BlockDispatcher is a component that is responsible for dispatching blocks to the correct engine instance or
// triggering a warp sync.
type BlockDispatcher struct {
	// protocol is the protocol instance that is using this BlockDispatcher instance.
	protocol *Protocol

	// dispatchWorkers is the worker pool that is used to dispatch blocks to the correct engine instance.
	dispatchWorkers *workerpool.WorkerPool

	// warpSyncWorkers is the worker pool that is used to process the WarpSync requests and responses.
	warpSyncWorkers *workerpool.WorkerPool

	// unsolidCommitmentBlocks is a buffer that stores blocks that have an unsolid slot commitment.
	unsolidCommitmentBlocks *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

	// pendingWarpSyncRequests is the set of pending requests that are waiting to be processed.
	pendingWarpSyncRequests *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// processedWarpSyncRequests is the set of processed requests.
	processedWarpSyncRequests ds.Set[iotago.CommitmentID]

	// shutdownEvent is a reactive event that is triggered when the BlockDispatcher instance is stopped.
	shutdownEvent reactive.Event
}

// NewBlockDispatcher creates a new BlockDispatcher instance.
func NewBlockDispatcher(protocol *Protocol, opts ...options.Option[BlockDispatcher]) *BlockDispatcher {
	return options.Apply(&BlockDispatcher{
		protocol:                  protocol,
		dispatchWorkers:           protocol.Workers.CreatePool("BlockDispatcher.Dispatch"),
		warpSyncWorkers:           protocol.Workers.CreatePool("BlockDispatcher.WarpSync", 1),
		unsolidCommitmentBlocks:   buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
		pendingWarpSyncRequests:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](WarpSyncRetryInterval)),
		processedWarpSyncRequests: ds.NewSet[iotago.CommitmentID](),
		shutdownEvent:             reactive.NewEvent(),
	}, opts, func(b *BlockDispatcher) {
		protocol.HookConstructed(b.initEngineMonitoring)
		protocol.HookInitialized(b.initNetworkConnection)
		protocol.HookShutdown(b.shutdown)
	})
}

// Dispatch dispatches the given block to the correct engine instance.
func (b *BlockDispatcher) Dispatch(block *model.Block, src peer.ID) error {
	slotCommitment := b.protocol.ChainManager.LoadCommitmentOrRequestMissing(block.ProtocolBlock().SlotCommitmentID)
	if !slotCommitment.SolidEvent().WasTriggered() {
		if !b.unsolidCommitmentBlocks.Add(slotCommitment.ID(), types.NewTuple(block, src)) {
			return ierrors.Errorf("failed to add block %s to unsolid commitment buffer", block.ID())
		}

		return ierrors.Errorf("failed to dispatch block %s: slot commitment %s is not solid", block.ID(), slotCommitment.ID())
	}

	matchingEngineFound := false
	for _, engine := range []*engine.Engine{b.protocol.MainEngineInstance(), b.protocol.CandidateEngineInstance()} {
		if engine != nil && (engine.ChainID() == slotCommitment.Chain().ForkingPoint.ID() || engine.BlockRequester.HasTicker(block.ID())) {
			if b.inSyncWindow(engine, block) {
				engine.ProcessBlockFromPeer(block, src)
			} else {
				// Stick too new blocks into the unsolid commitment buffer so that they can be dispatched once the
				// engine instance is in sync (mostly needed for tests).
				if !b.unsolidCommitmentBlocks.Add(slotCommitment.ID(), types.NewTuple(block, src)) {
					return ierrors.Errorf("failed to add block %s to unsolid commitment buffer", block.ID())
				}
			}

			matchingEngineFound = true
		}
	}

	if !matchingEngineFound {
		return ierrors.Errorf("failed to dispatch block %s: no matching engine found", block.ID())
	}

	return nil
}

// initEngineMonitoring initializes the automatic monitoring of the engine instances.
func (b *BlockDispatcher) initEngineMonitoring() {
	b.monitorLatestEngineCommitment(b.protocol.MainEngineInstance())

	b.protocol.EngineManager.OnEngineCreated(b.monitorLatestEngineCommitment)

	b.protocol.Events.ChainManager.CommitmentPublished.Hook(func(chainCommitment *chainmanager.ChainCommitment) {
		// as soon as a commitment is solid, it's chain is known and it can be dispatched
		chainCommitment.SolidEvent().OnTrigger(func() {
			b.runTask(func() {
				b.injectUnsolidCommitmentBlocks(chainCommitment.Commitment().ID())
			}, b.dispatchWorkers)

			b.runTask(func() {
				b.warpSyncIfNecessary(b.targetEngine(chainCommitment), chainCommitment)
			}, b.warpSyncWorkers)
		})
	})

	b.protocol.Events.Engine.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		b.runTask(func() {
			b.injectUnsolidCommitmentBlocks(commitment.ID())
		}, b.dispatchWorkers)
	})

	b.protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(b.evict)
}

// initNetworkConnection initializes the network connection of the BlockDispatcher instance.
func (b *BlockDispatcher) initNetworkConnection() {
	b.protocol.Events.Engine.BlockRequester.Tick.Hook(func(blockID iotago.BlockID) {
		b.runTask(func() {
			b.protocol.networkProtocol.RequestBlock(blockID)
		}, b.dispatchWorkers)
	})

	b.pendingWarpSyncRequests.Events.Tick.Hook(func(id iotago.CommitmentID) {
		b.runTask(func() {
			b.protocol.networkProtocol.SendWarpSyncRequest(id)
		}, b.dispatchWorkers)
	})

	b.protocol.Events.Network.BlockReceived.Hook(func(block *model.Block, src peer.ID) {
		b.runTask(func() {
			b.protocol.HandleError(b.Dispatch(block, src))
		}, b.dispatchWorkers)
	})

	b.protocol.Events.Network.WarpSyncRequestReceived.Hook(func(commitmentID iotago.CommitmentID, src peer.ID) {
		b.runTask(func() {
			b.protocol.HandleError(b.processWarpSyncRequest(commitmentID, src))
		}, b.warpSyncWorkers)
	})

	b.protocol.Events.Network.WarpSyncResponseReceived.Hook(func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, tangleMerkleProof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationMerkleProof *merklehasher.Proof[iotago.Identifier], src peer.ID) {
		b.runTask(func() {
			b.protocol.HandleError(b.processWarpSyncResponse(commitmentID, blockIDs, tangleMerkleProof, transactionIDs, mutationMerkleProof, src))
		}, b.warpSyncWorkers)
	})
}

// processWarpSyncRequest processes a WarpSync request.
func (b *BlockDispatcher) processWarpSyncRequest(commitmentID iotago.CommitmentID, src peer.ID) error {
	// TODO: check if the peer is allowed to request the warp sync

	committedSlot, err := b.protocol.MainEngineInstance().CommittedSlot(commitmentID)
	if err != nil {
		return ierrors.Wrapf(err, "failed to get slot %d (not committed yet)", commitmentID.Slot())
	}

	commitment, err := committedSlot.Commitment()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get commitment from slot %d", commitmentID.Slot())
	} else if commitment.ID() != commitmentID {
		return ierrors.Wrapf(err, "commitment ID mismatch: %s != %s", commitment.ID(), commitmentID)
	}

	blockIDs, err := committedSlot.BlockIDs()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get block IDs from slot %d", commitmentID.Slot())
	}

	transactionIDs, err := committedSlot.TransactionIDs()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get transaction IDs from slot %d", commitmentID.Slot())
	}

	roots, err := committedSlot.Roots()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get roots from slot %d", commitmentID.Slot())
	}

	b.protocol.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), transactionIDs, roots.MutationProof(), src)

	return nil
}

// processWarpSyncResponse processes a WarpSync response.
func (b *BlockDispatcher) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, tangleMerkleProof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationMerkleProof *merklehasher.Proof[iotago.Identifier], _ peer.ID) error {
	if b.processedWarpSyncRequests.Has(commitmentID) {
		return nil
	}

	// First we make sure that the commitment, the provided blockIDs with tangle proof and the provided transactionIDs with mutation proof are valid.
	chainCommitment, exists := b.protocol.ChainManager.Commitment(commitmentID)
	if !exists {
		return ierrors.Errorf("failed to get chain commitment for %s", commitmentID)
	}

	targetEngine := b.targetEngine(chainCommitment)
	if targetEngine == nil {
		return ierrors.Errorf("failed to get target engine for %s", commitmentID)
	}

	// Make sure that already evicted commitments are not processed. This might happen if there's a lot of slots to process ]
	// and old responses are still in the task queue.
	if loadedCommitment, err := targetEngine.Storage.Commitments().Load(commitmentID.Slot()); err == nil && loadedCommitment.ID() == commitmentID {
		return nil
	}

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		_ = acceptedBlocks.Add(blockID) // a mapdb can never return an error
	}

	if !iotago.VerifyProof(tangleMerkleProof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.Commitment().RootsID()) {
		return ierrors.Errorf("failed to verify tangle merkle proof for %s", commitmentID)
	}

	acceptedTransactionIDs := ads.NewSet[iotago.TransactionID](mapdb.NewMapDB(), iotago.TransactionID.Bytes, iotago.TransactionIDFromBytes)
	for _, transactionID := range transactionIDs {
		_ = acceptedTransactionIDs.Add(transactionID) // a mapdb can never return an error
	}

	if !iotago.VerifyProof(mutationMerkleProof, iotago.Identifier(acceptedTransactionIDs.Root()), chainCommitment.Commitment().RootsID()) {
		return ierrors.Errorf("failed to verify mutation merkle proof for %s", commitmentID)
	}

	b.pendingWarpSyncRequests.StopTicker(commitmentID)

	b.processedWarpSyncRequests.Add(commitmentID)

	// If the engine is "dirty" we need to restore the state of the engine to the state of the chain commitment.
	// As we already decided to switch and sync to this chain we should make sure that processing the blocks from the commitment
	// leads to the verified commitment.
	if targetEngine.Notarization.AcceptedBlocksCount(commitmentID.Slot()) > 0 {
		b.protocol.activeEngineMutex.Lock()

		newEngine, err := b.protocol.EngineManager.RollbackEngine(commitmentID.Slot() - 1)
		if err != nil {
			return ierrors.Wrapf(err, "failed to rollback engine for slot %d", commitmentID.Slot())
		}
		b.protocol.Events.MainEngineRestarted.Trigger(newEngine)

		newEngine.SetChainID(targetEngine.ChainID())

		if err := b.protocol.EngineManager.SetActiveInstance(newEngine); err != nil {
			return ierrors.Wrap(err, "failed to set active engine instance")
		}
		//
		b.protocol.linkToEngine(newEngine)
		//
		b.protocol.mainEngine.Shutdown()
		b.protocol.mainEngine = newEngine
		targetEngine = newEngine
		b.protocol.activeEngineMutex.Unlock()
	}

	// Once all blocks are booked we
	//   1. Mark all transactions as accepted
	//   2. Mark all blocks as accepted
	//   3. Force commitment of the slot
	totalBlocks := uint32(len(blockIDs))
	var bookedBlocks atomic.Uint32
	var notarizedBlocks atomic.Uint32

	forceCommitmentFunc := func() {
		// 3. Force commitment of the slot
		producedCommitment, err := targetEngine.Notarization.ForceCommit(commitmentID.Slot())
		if err != nil {
			b.protocol.HandleError(err)
			return
		}

		// 4. Verify that the produced commitment is the same as the initially requested one
		if producedCommitment.ID() != commitmentID {
			b.protocol.HandleError(ierrors.Errorf("producedCommitment ID mismatch: %s != %s", producedCommitment.ID(), commitmentID))
			return
		}
	}

	blockBookedFunc := func(_, _ bool) {
		if bookedBlocks.Add(1) != totalBlocks {
			return
		}

		// 1. Mark all transactions as accepted
		for _, transactionID := range transactionIDs {
			targetEngine.Ledger.ConflictDAG().SetAccepted(transactionID)
		}

		// 2. Mark all blocks as accepted
		for _, blockID := range blockIDs {
			block, exists := targetEngine.BlockCache.Block(blockID)
			if !exists { // this should never happen as we just booked these blocks in this slot.
				continue
			}

			targetEngine.BlockGadget.SetAccepted(block)

			block.Notarized().OnUpdate(func(_, _ bool) {
				// Wait for all blocks to be notarized before forcing the commitment of the slot.
				if notarizedBlocks.Add(1) != totalBlocks {
					return
				}

				forceCommitmentFunc()
			})
		}
	}

	if len(blockIDs) == 0 {
		forceCommitmentFunc()
		return nil
	}

	for _, blockID := range blockIDs {
		block, _ := targetEngine.BlockDAG.GetOrRequestBlock(blockID)
		if block == nil { // this should never happen as we're requesting the blocks for this slot so it can't be evicted.
			continue
		}

		// We need to make sure that we add all blocks as root blocks because we don't know which blocks are root blocks without
		// blocks from future slots. We're committing the current slot which then leads to the eviction of the blocks from the
		// block cache and thus if not root blocks no block in the next slot can become solid.
		targetEngine.EvictionState.AddRootBlock(block.ID(), block.SlotCommitmentID())

		block.Booked().OnUpdate(blockBookedFunc)
	}

	return nil
}

// inSyncWindow returns whether the given block is within the sync window of the given engine instance.
//
// We limit the amount of slots ahead of the latest commitment that we forward to the engine instance to prevent memory
// exhaustion while syncing.
func (b *BlockDispatcher) inSyncWindow(engine *engine.Engine, block *model.Block) bool {
	if engine.BlockRequester.HasTicker(block.ID()) {
		return true
	}

	slotCommitmentID := block.ProtocolBlock().SlotCommitmentID
	latestCommitmentSlot := engine.Storage.Settings().LatestCommitment().Slot()
	maxCommittableAge := engine.APIForSlot(slotCommitmentID.Slot()).ProtocolParameters().MaxCommittableAge()

	return block.ID().Slot() <= latestCommitmentSlot+maxCommittableAge
}

// warpSyncIfNecessary triggers a warp sync if necessary.
func (b *BlockDispatcher) warpSyncIfNecessary(e *engine.Engine, chainCommitment *chainmanager.ChainCommitment) {
	if e == nil || e.WasShutdown() {
		return
	}

	chain := chainCommitment.Chain()
	latestCommitmentSlot := e.Storage.Settings().LatestCommitment().Slot()

	// We don't want to warpsync if the latest commitment of the engine is very close to the latest commitment of the
	// chain as the node might just be about to commit it itself. This is important for tests, as we always need to issue
	// 2 slots ahead of the latest commitment of the chain to make sure that the other nodes can warp sync.
	if latestCommitmentSlot+1 >= chain.LatestCommitment().Commitment().Slot() {
		return
	}

	for slotToWarpSync := latestCommitmentSlot + 1; slotToWarpSync <= latestCommitmentSlot+1; slotToWarpSync++ {
		commitmentToSync := chain.Commitment(slotToWarpSync)
		if commitmentToSync == nil {
			break
		}

		if !b.processedWarpSyncRequests.Has(commitmentToSync.ID()) {
			b.pendingWarpSyncRequests.StartTicker(commitmentToSync.ID())
		}
	}
}

// injectUnsolidCommitmentBlocks injects the unsolid blocks for the given commitment ID into the correct engine
// instance.
func (b *BlockDispatcher) injectUnsolidCommitmentBlocks(id iotago.CommitmentID) {
	for _, tuple := range b.unsolidCommitmentBlocks.GetValues(id) {
		b.protocol.HandleError(b.Dispatch(tuple.A, tuple.B))
	}
}

// targetEngine returns the engine instance that should be used for the given commitment.
func (b *BlockDispatcher) targetEngine(commitment *chainmanager.ChainCommitment) *engine.Engine {
	if chain := commitment.Chain(); chain != nil {
		chainID := chain.ForkingPoint.Commitment().ID()

		if engine := b.protocol.MainEngineInstance(); engine.ChainID() == chainID {
			return engine
		}

		if engine := b.protocol.CandidateEngineInstance(); engine != nil && engine.ChainID() == chainID {
			return engine
		}
	}

	return nil
}

// monitorLatestEngineCommitment monitors the latest commitment of the given engine instance and triggers a warp sync if
// necessary.
func (b *BlockDispatcher) monitorLatestEngineCommitment(engineInstance *engine.Engine) {
	unsubscribe := engineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		if latestEngineCommitment, exists := b.protocol.ChainManager.Commitment(commitment.ID()); exists {
			b.processedWarpSyncRequests.Delete(commitment.ID())

			b.warpSyncIfNecessary(engineInstance, latestEngineCommitment)
		}
	}).Unhook

	engineInstance.HookStopped(unsubscribe)
}

// evict evicts all elements from the unsolid commitment blocks buffer and the pending warp sync requests that are older
// than the given index.
func (b *BlockDispatcher) evict(slot iotago.SlotIndex) {
	b.pendingWarpSyncRequests.EvictUntil(slot)
	b.unsolidCommitmentBlocks.EvictUntil(slot)
}

// shutdown shuts down the BlockDispatcher instance.
func (b *BlockDispatcher) shutdown() {
	b.shutdownEvent.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			b.pendingWarpSyncRequests.Shutdown()

			b.dispatchWorkers.Shutdown(true).ShutdownComplete.Wait()
			b.warpSyncWorkers.Shutdown(true).ShutdownComplete.Wait()
		}

		return true
	})
}

// runTask runs the given task on the given worker pool if the BlockDispatcher instance is not shutdown.
func (b *BlockDispatcher) runTask(task func(), pool *workerpool.WorkerPool) {
	b.shutdownEvent.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			pool.Submit(task)
		}

		return isShutdown
	})
}

// WarpSyncRetryInterval is the interval in which a warp sync request is retried.
const WarpSyncRetryInterval = 1 * time.Minute
