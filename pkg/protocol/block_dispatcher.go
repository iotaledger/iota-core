package protocol

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// BlockDispatcher is a component that is responsible for dispatching blocks to the correct engine instance.
type BlockDispatcher struct {
	// protocol is the protocol instance that is using this BlockDispatcher instance.
	protocol *Protocol

	// dispatchWorkers is the worker pool that is used to dispatch blocks to the correct engine instance.
	dispatchWorkers *workerpool.WorkerPool

	// warpSyncWorkers is the worker pool that is used to process the WarpSync requests and responses.
	warpSyncWorkers *workerpool.WorkerPool

	// unsolidCommitmentBlocks is a buffer that stores blocks that have an unsolid slot commitment.
	unsolidCommitmentBlocks *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, network.PeerID]]

	// pendingWarpSyncRequests is the set of pending requests that are waiting to be processed.
	pendingWarpSyncRequests *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// processedWarpSyncRequests is the set of processed requests.
	processedWarpSyncRequests ds.Set[iotago.CommitmentID]

	// shutdownEvent is a reactive event that is triggered when the BlockDispatcher instance is stopped.
	shutdownEvent reactive.Event
}

// NewBlockDispatcher creates a new BlockDispatcher instance.
func NewBlockDispatcher(protocol *Protocol) *BlockDispatcher {
	b := &BlockDispatcher{
		protocol:                  protocol,
		dispatchWorkers:           protocol.Workers.CreatePool("BlockDispatcher.Dispatch"),
		warpSyncWorkers:           protocol.Workers.CreatePool("BlockDispatcher.WarpSync", 1),
		unsolidCommitmentBlocks:   buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, network.PeerID]](20, 100),
		pendingWarpSyncRequests:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](WarpSyncRetryInterval)),
		processedWarpSyncRequests: ds.NewSet[iotago.CommitmentID](),
		shutdownEvent:             reactive.NewEvent(),
	}

	protocol.HookConstructed(b.initEngineMonitoring)
	protocol.HookInitialized(b.initNetworkConnection)
	protocol.HookStopped(b.shutdown)

	return b
}

func (b *BlockDispatcher) Dispatch(block *model.Block, src network.PeerID) error {
	slotCommitment := b.protocol.ChainManager.LoadCommitmentOrRequestMissing(block.ProtocolBlock().SlotCommitmentID)
	if !slotCommitment.Solid().Get() {
		if !b.unsolidCommitmentBlocks.Add(slotCommitment.ID(), types.NewTuple(block, src)) {
			return ierrors.Errorf("protocol Dispatch failed. chain is not solid and could not add to unsolid slotCommitment buffer: slotcommitment: %s, block ID: %s", slotCommitment.ID(), block.ID())
		}

		return ierrors.Errorf("protocol Dispatch failed. chain is not solid: slotcommitment: %s, block ID: %s", slotCommitment.ID(), block.ID())
	}

	matchingEngineFound := false
	for _, engine := range []*engine.Engine{b.protocol.MainEngineInstance(), b.protocol.CandidateEngineInstance()} {
		if engine != nil && (engine.ChainID() == slotCommitment.Chain().ForkingPoint.ID() || engine.BlockRequester.HasTicker(block.ID())) {
			if !b.inWarpSyncRange(engine, block) {
				engine.ProcessBlockFromPeer(block, src)
			}

			matchingEngineFound = true
		}
	}

	if !matchingEngineFound {
		return ierrors.Errorf("block from source %s was not processed: %s; commits to: %s", src, block.ID(), slotCommitment.ID())
	}

	return nil
}

func (b *BlockDispatcher) ShutdownEvent() reactive.Event {
	return b.shutdownEvent
}

func (b *BlockDispatcher) initEngineMonitoring() {
	b.monitorLatestCommitmentUpdated(b.protocol.mainEngine)

	b.protocol.engineManager.OnEngineCreated(b.monitorLatestCommitmentUpdated)

	b.protocol.Events.ChainManager.CommitmentPublished.Hook(func(chainCommitment *chainmanager.ChainCommitment) {
		chainCommitment.Solid().OnTrigger(func() {
			b.injectUnsolidCommitmentBlocks(chainCommitment.Commitment().ID())
			b.warpSyncIfNecessary(b.targetEngine(chainCommitment), chainCommitment)
		})
	})

	b.protocol.Events.Engine.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		b.injectUnsolidCommitmentBlocks(commitment.ID())
	}, event.WithWorkerPool(b.dispatchWorkers))

	b.protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(b.evict)
}

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

	b.protocol.Events.Network.BlockReceived.Hook(func(block *model.Block, src network.PeerID) {
		b.runTask(func() {
			b.protocol.HandleError(b.Dispatch(block, src))
		}, b.dispatchWorkers)
	})

	b.protocol.Events.Network.WarpSyncRequestReceived.Hook(func(commitmentID iotago.CommitmentID, src network.PeerID) {
		b.runTask(func() {
			b.protocol.HandleError(b.processWarpSyncRequest(commitmentID, src))
		}, b.warpSyncWorkers)
	})

	b.protocol.Events.Network.WarpSyncResponseReceived.Hook(func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier], src network.PeerID) {
		b.runTask(func() {
			b.protocol.HandleError(b.processWarpSyncResponse(commitmentID, blockIDs, merkleProof, src))
		}, b.warpSyncWorkers)
	})
}

// inWarpSyncRange returns whether the given block should be processed by the BlockDispatcher instance instead of the engine.
//
// This is the case if the block is more than a warp sync threshold ahead of the latest commitment while also committing
// to an unknown slot that can potentially be warp synced.
func (b *BlockDispatcher) inWarpSyncRange(engine *engine.Engine, block *model.Block) bool {
	if engine.BlockRequester.HasTicker(block.ID()) {
		return false
	}

	slotCommitmentID := block.ProtocolBlock().SlotCommitmentID
	latestCommitment := engine.Storage.Settings().LatestCommitment()
	maxCommittableAge := engine.APIForSlot(slotCommitmentID.Index()).ProtocolParameters().MaxCommittableAge()

	shouldProcess := block.ID().Index() > latestCommitment.Index()+2*maxCommittableAge && slotCommitmentID.Index() > latestCommitment.Index()

	if shouldProcess {
		fmt.Println(">> Should process: latest", latestCommitment.Index(), "committing to", slotCommitmentID.Index(), "MCA", maxCommittableAge)
	}

	return shouldProcess
}

func (b *BlockDispatcher) runTask(task func(), pool *workerpool.WorkerPool) {
	b.shutdownEvent.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			pool.Submit(task)
		}

		return isShutdown
	})
}

func (b *BlockDispatcher) processWarpSyncRequest(commitmentID iotago.CommitmentID, src network.PeerID) error {
	// TODO: check if the peer is allowed to request the warp sync

	committedSlot, err := b.protocol.MainEngineInstance().CommittedSlot(commitmentID.Index())
	if err != nil {
		return ierrors.Wrapf(err, "failed to get slot %d (not committed yet)", commitmentID.Index())
	}

	commitment, err := committedSlot.Commitment()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get commitment from slot %d", commitmentID.Index())
	} else if commitment.ID() != commitmentID {
		return ierrors.Wrapf(err, "commitment ID mismatch: %s != %s", commitment.ID(), commitmentID)
	}

	blockIDs, err := committedSlot.BlockIDs()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get block IDs from slot %d", commitmentID.Index())
	}

	roots, err := committedSlot.Roots()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get roots from slot %d", commitmentID.Index())
	}

	b.protocol.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), src)

	return nil
}

func (b *BlockDispatcher) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) error {
	if b.processedWarpSyncRequests.Has(commitmentID) {
		return nil
	}

	chainCommitment, exists := b.protocol.ChainManager.Commitment(commitmentID)
	if !exists {
		return ierrors.Errorf("failed to get chain commitment for %s", commitmentID)
	}

	targetEngine := b.targetEngine(chainCommitment)
	if targetEngine == nil {
		return ierrors.Errorf("failed to get target engine for %s", commitmentID)
	}

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
	}

	if !iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.Commitment().RootsID()) {
		return ierrors.Errorf("failed to verify merkle proof for %s", commitmentID)
	}

	b.pendingWarpSyncRequests.StopTicker(commitmentID)

	b.processedWarpSyncRequests.Add(commitmentID)

	targetEngine.Events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
		block.ID()
	})

	for _, blockID := range blockIDs {
		targetEngine.BlockDAG.GetOrRequestBlock(blockID)
	}

	return nil
}

func (b *BlockDispatcher) warpSyncIfNecessary(e *engine.Engine, chainCommitment *chainmanager.ChainCommitment) {
	if e == nil || chainCommitment == nil {
		return
	}

	chain := chainCommitment.Chain()
	maxCommittableAge := e.APIForSlot(chainCommitment.Commitment().Index()).ProtocolParameters().MaxCommittableAge()
	minCommittableAge := e.APIForSlot(chainCommitment.Commitment().Index()).ProtocolParameters().MinCommittableAge()
	latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Index()

	if chainCommitment.Commitment().Index() > latestCommitmentIndex+maxCommittableAge {
		for slotToWarpSync := latestCommitmentIndex + 1; slotToWarpSync <= latestCommitmentIndex+minCommittableAge+2; slotToWarpSync++ {
			if commitmentToSync := chain.Commitment(slotToWarpSync); commitmentToSync != nil && !b.processedWarpSyncRequests.Has(commitmentToSync.ID()) {
				b.pendingWarpSyncRequests.StartTicker(commitmentToSync.ID())
			}
		}
	}
}

func (b *BlockDispatcher) injectUnsolidCommitmentBlocks(id iotago.CommitmentID) {
	for _, tuple := range b.unsolidCommitmentBlocks.GetValues(id) {
		b.Dispatch(tuple.A, tuple.B)
	}
}

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

func (b *BlockDispatcher) monitorLatestCommitmentUpdated(engineInstance *engine.Engine) {
	engineInstance.HookStopped(engineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		fmt.Println(">> monitorLatestCommitmentUpdated Committed", commitment.Index())
		chainCommitment, exists := b.protocol.ChainManager.Commitment(commitment.ID())
		if !exists {
			return
		}

		b.processedWarpSyncRequests.Delete(commitment.ID())

		b.warpSyncIfNecessary(engineInstance, chainCommitment)
	}).Unhook)
}

func (b *BlockDispatcher) evict(index iotago.SlotIndex) {
	b.pendingWarpSyncRequests.EvictUntil(index)
	b.unsolidCommitmentBlocks.EvictUntil(index)
}

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

const (
	WarpSyncRetryInterval = 1 * time.Minute
)
