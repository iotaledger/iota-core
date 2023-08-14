package protocol

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// WarpSync is a protocol that is responsible for syncing the node with the network by requesting the content of slots.
type WarpSync struct {
	// protocol is the protocol instance that is using this WarpSync instance.
	protocol *Protocol

	// workerPool is the worker pool that is used to process the requests and responses.
	workerPool *workerpool.WorkerPool

	// pendingRequests is the set of pending requests that are waiting to be processed.
	pendingRequests *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// processedRequests is the set of processed requests.
	processedRequests ds.Set[iotago.CommitmentID]

	// isShutdown is a reactive event that is triggered when the WarpSync instance is shutdown.
	isShutdown reactive.Event
}

// NewWarpSync creates a new WarpSync instance.
func NewWarpSync(protocol *Protocol) *WarpSync {
	w := &WarpSync{
		protocol:          protocol,
		workerPool:        protocol.Workers.CreatePool("WarpSyncManager", 1),
		pendingRequests:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](WarpSyncRetryInterval)),
		processedRequests: ds.NewSet[iotago.CommitmentID](),
		isShutdown:        reactive.NewEvent(),
	}

	protocol.HookConstructed(func() {
		w.monitorLatestCommitmentUpdated(protocol.mainEngine)

		protocol.engineManager.OnEngineCreated(w.monitorLatestCommitmentUpdated)

		protocol.ChainManager.Events.CommitmentPublished.Hook(func(chainCommitment *chainmanager.ChainCommitment) {
			chainCommitment.Solid().OnTrigger(func() {
				w.warpSyncIfNecessary(w.targetEngine(chainCommitment), chainCommitment)
			})
		})
	})

	protocol.HookInitialized(func() {
		w.pendingRequests.Events.Tick.Hook(func(id iotago.CommitmentID) {
			fmt.Println("Sending WarpSyncRequest for", id)
			protocol.networkProtocol.SendWarpSyncRequest(id)
		})

		protocol.Events.Started.Hook(func() {
			protocol.Events.Network.WarpSyncRequestReceived.Hook(w.processRequest)
			protocol.Events.Network.WarpSyncResponseReceived.Hook(w.processResponse)
		})
	})

	protocol.HookStopped(w.shutdown)

	return w
}

func (w *WarpSync) ProcessBlock(block *model.Block, slotCommitment *chainmanager.ChainCommitment, src network.PeerID) error {
	err := ierrors.Errorf("block from source %s was not processed: %s; commits to: %s", src, block.ID(), slotCommitment.ID())

	for _, engine := range []*engine.Engine{w.protocol.MainEngineInstance(), w.protocol.CandidateEngineInstance()} {
		if engine != nil && (engine.ChainID() == slotCommitment.Chain().ForkingPoint.ID() || engine.BlockRequester.HasTicker(block.ID())) {
			err = nil

			if !w.shouldProcess(engine, block) {
				engine.ProcessBlockFromPeer(block, src)
			}
		}
	}

	return err
}

func (w *WarpSync) IsShutdown() reactive.Event {
	return w.isShutdown
}

// shouldProcess returns whether the given block should be processed by the WarpSync instance instead of the engine.
//
// This is the case if the block is more than a warp sync threshold ahead of the latest commitment while also committing
// to an unknown slot that can potentially be warp synced.
func (w *WarpSync) shouldProcess(engine *engine.Engine, block *model.Block) bool {
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

func (w *WarpSync) processRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	w.isShutdown.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			fmt.Println("WarpSyncManager.ProcessRequest: received request from", src)

			w.workerPool.Submit(func() {
				w.processRequestSync(commitmentID, src)
			})
		}

		return isShutdown
	})
}

func (w *WarpSync) processResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	fmt.Println(">> warpSync PROCESSRESPONSE", commitmentID)
	w.isShutdown.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			fmt.Println("WarpSyncManager.ProcessResponse: received response for", commitmentID)

			w.workerPool.Submit(func() {
				w.processResponseSync(commitmentID, blockIDs, merkleProof)
			})
		}

		return isShutdown
	})
}

func (w *WarpSync) processRequestSync(commitmentID iotago.CommitmentID, src network.PeerID) {
	// TODO: check if the peer is allowed to request the warp sync

	committedSlot, err := w.protocol.MainEngineInstance().CommittedSlot(commitmentID.Index())
	if err != nil {
		fmt.Println("WarpSyncManager.ProcessRequest: err != nil", err)
		return
	}

	commitment, err := committedSlot.Commitment()
	if err != nil || commitment.ID() != commitmentID {
		fmt.Println("WarpSyncManager.ProcessRequest: commitment == nil")
		return
	}

	blockIDs, err := committedSlot.BlockIDs()
	if err != nil {
		fmt.Println("WarpSyncManager.ProcessRequest: blockIDs == nil")
		return
	}

	roots, err := committedSlot.Roots()
	if err != nil {
		fmt.Println("WarpSyncManager.ProcessRequest: roots == nil")
		return
	}

	fmt.Println("WarpSyncManager.ProcessRequest: sending response to", src, "for", commitmentID, "with", len(blockIDs), "blocks")

	w.protocol.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), src)
}

func (w *WarpSync) processResponseSync(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier]) {
	if w.processedRequests.Has(commitmentID) {
		fmt.Println("WarpSyncManager.ProcessResponse: already processed", commitmentID)
		return
	}

	chainCommitment, exists := w.protocol.ChainManager.Commitment(commitmentID)
	if !exists {
		fmt.Println("WarpSyncManager.ProcessResponse: chainCommitment == nil")
		return
	}

	targetEngine := w.targetEngine(chainCommitment)
	if targetEngine == nil {
		fmt.Println("WarpSyncManager.ProcessResponse: targetEngine == nil")
		return
	}

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
	}

	if !iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.Commitment().RootsID()) {
		fmt.Println("WarpSyncManager.ProcessResponse: !iotago.VerifyProof")
		return
	}

	w.pendingRequests.StopTicker(commitmentID)

	w.processedRequests.Add(commitmentID)

	fmt.Println("WarpSyncManager.ProcessResponse: PROCESS FEEDING", commitmentID.Index())
	for _, blockID := range blockIDs {
		targetEngine.BlockDAG.GetOrRequestBlock(blockID)
	}
}

func (w *WarpSync) warpSyncIfNecessary(e *engine.Engine, chainCommitment *chainmanager.ChainCommitment) {
	fmt.Println("WarpSyncManager.warpSyncIfNecessary")

	if e == nil || chainCommitment == nil {
		return
	}

	chain := chainCommitment.Chain()
	maxCommittableAge := e.APIForSlot(chainCommitment.Commitment().Index()).ProtocolParameters().MaxCommittableAge()
	latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Index()

	fmt.Println("WarpSyncManager.warpSyncIfNecessary: latest", latestCommitmentIndex, "chainCommitment", chainCommitment.Commitment().Index())

	if chainCommitment.Commitment().Index() <= latestCommitmentIndex+maxCommittableAge {
		fmt.Println("WarpsyncManager.warpSyncIfNecessary: too close!", chainCommitment.Commitment().Index())
		return
	}

	for slotToWarpSync := latestCommitmentIndex + 1; slotToWarpSync <= latestCommitmentIndex+maxCommittableAge; slotToWarpSync++ {
		commitmentToSync := chain.Commitment(slotToWarpSync)
		if commitmentToSync == nil {
			fmt.Println("WarpSyncManager.warpSyncIfNecessary: commitmentToSync == nil")
			return
		}

		if w.processedRequests.Has(commitmentToSync.ID()) {
			continue
		}

		fmt.Println("WarpSyncManager.warpSyncIfNecessary: WarpSyncing", commitmentToSync.ID())
		w.pendingRequests.StartTicker(commitmentToSync.ID())
	}
}

func (w *WarpSync) targetEngine(commitment *chainmanager.ChainCommitment) *engine.Engine {
	if chain := commitment.Chain(); chain != nil {
		chainID := chain.ForkingPoint.Commitment().ID()

		if engine := w.protocol.MainEngineInstance(); engine.ChainID() == chainID {
			return engine
		}

		if engine := w.protocol.CandidateEngineInstance(); engine != nil && engine.ChainID() == chainID {
			return engine
		}
	}

	return nil
}

func (w *WarpSync) monitorLatestCommitmentUpdated(engineInstance *engine.Engine) {
	engineInstance.HookStopped(engineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		fmt.Println(">> monitorLatestCommitmentUpdated Committed", commitment.Index())
		chainCommitment, exists := w.protocol.ChainManager.Commitment(commitment.ID())
		if !exists {
			return
		}

		w.processedRequests.Delete(commitment.ID())

		w.warpSyncIfNecessary(engineInstance, chainCommitment)
	}).Unhook)
}

func (w *WarpSync) shutdown() {
	w.isShutdown.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			w.pendingRequests.Shutdown()

			w.workerPool.Shutdown(true).ShutdownComplete.Wait()
		}

		return true
	})
}

const (
	WarpSyncRetryInterval = 1 * time.Minute
)
