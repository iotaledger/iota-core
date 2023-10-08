package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type WarpSyncProtocol struct {
	protocol   *Protocol
	workerPool *workerpool.WorkerPool
	ticker     *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	log.Logger
}

func NewWarpSyncProtocol(protocol *Protocol) *WarpSyncProtocol {
	c := &WarpSyncProtocol{
		Logger:     lo.Return1(protocol.Logger.NewChildLogger("WarpSync")),
		protocol:   protocol,
		workerPool: protocol.Workers.CreatePool("WarpSync"),
		ticker:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
	}

	c.ticker.Events.Tick.Hook(c.SendRequest)

	protocol.HookConstructed(func() {
		c.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.RequestBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
				if warpSyncBlocks {
					c.ticker.StartTicker(commitment.ID())
				} else {
					c.ticker.StopTicker(commitment.ID())
				}
			})
		})
	})

	return c
}

func (w *WarpSyncProtocol) SendRequest(commitmentID iotago.CommitmentID) {
	w.workerPool.Submit(func() {
		if commitment, err := w.protocol.Commitment(commitmentID, false); err == nil {
			w.protocol.Network.SendWarpSyncRequest(commitmentID)

			w.LogDebug("sent request", "commitment", commitment.LogName())
		}
	})
}

func (w *WarpSyncProtocol) SendResponse(commitment *Commitment, blockIDs iotago.BlockIDs, roots *iotago.Roots, to peer.ID) {
	w.workerPool.Submit(func() {
		w.protocol.Network.SendWarpSyncResponse(commitment.ID(), blockIDs, roots.TangleProof(), to)

		w.LogTrace("sent response", "commitment", commitment.LogName(), "toPeer", to)
	})
}

func (w *WarpSyncProtocol) ProcessResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	w.workerPool.Submit(func() {
		commitment, err := w.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				w.LogError("failed to load commitment for response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				w.LogTrace("failed to load commitment for response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		targetEngine := commitment.Engine.Get()
		if targetEngine == nil {
			w.LogDebug("failed to get target engine for response", "commitment", commitment.LogName())

			return
		}

		commitment.RequestedBlocksReceived.Compute(func(requestedBlocksReceived bool) bool {
			if requestedBlocksReceived || !commitment.RequestBlocks.Get() {
				w.LogTrace("response for already synced commitment", "commitment", commitment.LogName(), "fromPeer", from)

				return requestedBlocksReceived
			}

			acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
			for _, blockID := range blockIDs {
				_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
			}

			if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), commitment.RootsID()) {
				w.LogError("failed to verify merkle proof", "commitment", commitment.LogName(), "blockIDs", blockIDs, "proof", proof, "fromPeer", from)

				return false
			}

			w.ticker.StopTicker(commitmentID)

			for _, blockID := range blockIDs {
				targetEngine.BlockDAG.GetOrRequestBlock(blockID)
			}

			w.LogDebug("received response", "commitment", commitment.LogName())

			return true
		})
	})
}

func (w *WarpSyncProtocol) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	w.workerPool.Submit(func() {
		commitment, err := w.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				w.LogError("failed to load commitment for warp-sync request", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				w.LogTrace("failed to load commitment for warp-sync request", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		chain := commitment.Chain.Get()
		if chain == nil {
			w.LogTrace("warp-sync request for unsolid commitment", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		engineInstance := commitment.Engine.Get()
		if engineInstance == nil {
			w.LogTrace("warp-sync request for chain without engine", "chain", chain.LogName(), "fromPeer", from)

			return
		}

		committedSlot, err := engineInstance.CommittedSlot(commitmentID)
		if err != nil {
			w.LogTrace("warp-sync request for uncommitted slot", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			w.LogTrace("failed to get block ids for warp-sync request", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from, "err", err)

			return
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			w.LogTrace("failed to get roots for warp-sync request", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from, "err", err)

			return
		}

		w.SendResponse(commitment, blockIDs, roots, from)
	})
}

func (w *WarpSyncProtocol) Shutdown() {
	w.ticker.Shutdown()
	w.workerPool.Shutdown().ShutdownComplete.Wait()
}
