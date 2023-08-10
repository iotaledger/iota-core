package protocol

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

const (
	WarpSyncThreshold     = iotago.SlotIndex(5)
	WarpSyncRetryInterval = 1 * time.Minute
)

type WarpSyncManager struct {
	protocol                  *Protocol
	workers                   *workerpool.WorkerPool
	requester                 *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	verifiedWarpSyncResponses ds.Set[iotago.CommitmentID]
	warpSyncResponseMutex     *syncutils.DAGMutex[iotago.CommitmentID]
}

func NewWarpSyncManager(protocol *Protocol) *WarpSyncManager {
	w := &WarpSyncManager{
		protocol:              protocol,
		workers:               protocol.Workers.CreatePool("WarpSyncManager", 1),
		requester:             eventticker.New[iotago.SlotIndex, iotago.CommitmentID](eventticker.RetryInterval[iotago.SlotIndex, iotago.CommitmentID](WarpSyncRetryInterval)),
		warpSyncResponseMutex: syncutils.NewDAGMutex[iotago.CommitmentID](),
	}

	w.protocol.ChainManager.Events.CommitmentPublished.Hook(func(chainCommitment *chainmanager.ChainCommitment) {
		chainCommitment.IsSolid().OnTrigger(func() {
			w.warpSyncIfNecessary(w.targetEngine(chainCommitment), chainCommitment)
		})
	})

	return w
}

func (w *WarpSyncManager) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs []iotago.BlockID, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	w.workers.Submit(func() {
		w.warpSyncResponseMutex.Lock(commitmentID)
		defer w.warpSyncResponseMutex.Unlock(commitmentID)

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

		w.requester.StopTicker(commitmentID)

		for _, blockID := range blockIDs {
			targetEngine.BlockDAG.GetOrRequestBlock(blockID)
		}
	})
}

func (w *WarpSyncManager) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	w.workers.Submit(func() {
		committedSlot, err := w.protocol.MainEngineInstance().CommittedSlot(commitmentID.Index())
		if err != nil {
			fmt.Println("WarpSyncManager.ProcessWarpSyncRequest: committedSlot == nil")
			return
		}

		commitment, err := committedSlot.Commitment()
		if err != nil || commitment.ID() != commitmentID {
			fmt.Println("WarpSyncManager.ProcessWarpSyncRequest: commitment == nil")
			return
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			fmt.Println("WarpSyncManager.ProcessWarpSyncRequest: blockIDs == nil")
			return
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			fmt.Println("WarpSyncManager.ProcessWarpSyncRequest: roots == nil")
			return
		}

		w.protocol.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.AttestationsProof(), src)
	})
}

func (w *WarpSyncManager) MonitorEngine(engineInstance *engine.Engine) {
	engineInstance.HookStopped(engineInstance.Events.Notarization.LatestCommitmentUpdated.Hook(func(commitment *model.Commitment) {
		chainCommitment, exists := w.protocol.ChainManager.Commitment(commitment.ID())
		if !exists {
			return
		}

		warpSyncCommitment := chainCommitment.Chain().Commitment(commitment.Index() + WarpSyncThreshold)
		if warpSyncCommitment != nil {
			w.requester.StartTicker(warpSyncCommitment.ID())
		}
	}).Unhook)
}

func (w *WarpSyncManager) Shutdown() {
	w.requester.Shutdown()

	w.workers.Shutdown(true).ShutdownComplete.Wait()
}

func (w *WarpSyncManager) warpSyncIfNecessary(e *engine.Engine, chainCommitment *chainmanager.ChainCommitment) {
	if e == nil {
		return
	}

	chain := chainCommitment.Chain()

	latestCommitmentIndex := e.Storage.Settings().LatestCommitment().Index()
	if chainCommitment.Commitment().Index() <= latestCommitmentIndex+WarpSyncThreshold {
		return
	}

	for slotToWarpSync := latestCommitmentIndex + 1; slotToWarpSync <= latestCommitmentIndex+WarpSyncThreshold; slotToWarpSync++ {
		commitmentToSync := chain.Commitment(slotToWarpSync)
		if commitmentToSync == nil {
			fmt.Println("WarpSyncManager.warpSyncIfNecessary: commitmentToSync == nil")
			return
		}

		w.requester.StartTicker(commitmentToSync.ID())
	}
}

func (w *WarpSyncManager) targetEngine(commitment *chainmanager.ChainCommitment) *engine.Engine {
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
