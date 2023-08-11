package protocol

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
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
			chainCommitment.IsSolid().OnTrigger(func() {
				w.warpSyncIfNecessary(w.targetEngine(chainCommitment), chainCommitment)
			})
		})
	})

	protocol.HookInitialized(func() {
		w.pendingRequests.Events.Tick.Hook(func(id iotago.CommitmentID) {
			protocol.networkProtocol.SendWarpSyncRequest(id)
		})

		protocol.Events.Started.Hook(func() {
			protocol.Events.Network.WarpSyncRequestReceived.Hook(w.ProcessRequest)
			protocol.Events.Network.WarpSyncResponseReceived.Hook(w.ProcessResponse)
		})
	})

	return w
}

func (w *WarpSync) ProcessRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	w.isShutdown.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			w.workerPool.Submit(func() {
				w.processWarpSyncRequest(commitmentID, src)
			})
		}

		return isShutdown
	})
}

func (w *WarpSync) ProcessResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	w.isShutdown.Compute(func(isShutdown bool) bool {
		if !isShutdown {
			w.workerPool.Submit(func() {
				w.processWarpSyncResponse(commitmentID, blockIDs, merkleProof)
			})
		}

		return isShutdown
	})
}

func (w *WarpSync) Threshold(mainEngine *engine.Engine, index iotago.SlotIndex) iotago.SlotIndex {
	return mainEngine.Storage.Settings().LatestCommitment().Index() + mainEngine.APIForSlot(index).ProtocolParameters().MaxCommittableAge()
}

func (w *WarpSync) monitorLatestCommitmentUpdated(engineInstance *engine.Engine) {
	engineInstance.HookStopped(engineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		chainCommitment, exists := w.protocol.ChainManager.Commitment(commitment.ID())
		if !exists {
			return
		}

		w.processedRequests.Delete(commitment.ID())

		maxCommittableAge := engineInstance.APIForSlot(commitment.Index()).ProtocolParameters().MaxCommittableAge()
		warpSyncCommitment := chainCommitment.Chain().Commitment(commitment.Index() + maxCommittableAge)
		if warpSyncCommitment != nil {
			w.pendingRequests.StartTicker(warpSyncCommitment.ID())
		}
	}).Unhook)
}

func (w *WarpSync) Shutdown() {
	w.pendingRequests.Shutdown()

	w.workerPool.Shutdown(true).ShutdownComplete.Wait()
}

func (w *WarpSync) IsShutdown() reactive.Event {
	return w.isShutdown
}

func (w *WarpSync) processWarpSyncRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	// TODO: check if the peer is allowed to request the warp sync

	committedSlot, err := w.protocol.MainEngineInstance().CommittedSlot(commitmentID.Index())
	if err != nil {
		fmt.Println("WarpSyncManager.ProcessRequest: committedSlot == nil")
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

	w.protocol.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.AttestationsProof(), src)
}

func (w *WarpSync) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier]) {
	if w.processedRequests.Has(commitmentID) {
		return
	}

	chainCommitment, exists := w.protocol.ChainManager.Commitment(commitmentID)
	if !exists {
		return
	}

	targetEngine := w.targetEngine(chainCommitment)
	if targetEngine == nil {
		return
	}

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
	}

	if !iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.Commitment().RootsID()) {
		return
	}

	w.pendingRequests.StopTicker(commitmentID)

	w.processedRequests.Add(commitmentID)

	for _, blockID := range blockIDs {
		targetEngine.BlockDAG.GetOrRequestBlock(blockID)
	}
}

func (w *WarpSync) warpSyncIfNecessary(e *engine.Engine, chainCommitment *chainmanager.ChainCommitment) {
	if e == nil || chainCommitment == nil {
		return
	}

	chain := chainCommitment.Chain()
	maxCommittableAge := e.APIForSlot(chainCommitment.Commitment().Index()).ProtocolParameters().MaxCommittableAge()
	latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Index()

	if chainCommitment.Commitment().Index() <= latestCommitmentIndex+maxCommittableAge {
		fmt.Println("WarpsyncManager.warpSyncIfNecessary: too close!")
		return
	}

	for slotToWarpSync := latestCommitmentIndex + 1; slotToWarpSync <= latestCommitmentIndex+maxCommittableAge; slotToWarpSync++ {
		commitmentToSync := chain.Commitment(slotToWarpSync)
		if commitmentToSync == nil {
			fmt.Println("WarpSyncManager.warpSyncIfNecessary: commitmentToSync == nil")
			return
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

const (
	WarpSyncRetryInterval = 1 * time.Minute
)
