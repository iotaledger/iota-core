package protocol

import (
	"sync/atomic"

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

	protocol.Constructed.OnTrigger(func() {
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

func (w *WarpSyncProtocol) SendResponse(commitment *Commitment, blockIDs iotago.BlockIDs, roots *iotago.Roots, transactionIDs iotago.TransactionIDs, to peer.ID) {
	w.workerPool.Submit(func() {
		w.protocol.Network.SendWarpSyncResponse(commitment.ID(), blockIDs, roots.TangleProof(), transactionIDs, roots.MutationProof(), to)

		w.LogTrace("sent response", "commitment", commitment.LogName(), "toPeer", to)
	})
}

func (w *WarpSyncProtocol) ProcessResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationProof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
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

			acceptedBlocks := ads.NewSet[iotago.Identifier](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.BlockIDFromBytes)
			for _, blockID := range blockIDs {
				_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
			}

			if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), commitment.RootsID()) {
				w.LogError("failed to verify blocks proof", "commitment", commitment.LogName(), "blockIDs", blockIDs, "proof", proof, "fromPeer", from)

				return false
			}

			acceptedTransactionIDs := ads.NewSet[iotago.Identifier](mapdb.NewMapDB(), iotago.TransactionID.Bytes, iotago.TransactionIDFromBytes)
			for _, transactionID := range transactionIDs {
				_ = acceptedTransactionIDs.Add(transactionID) // a mapdb can never return an error
			}

			if !iotago.VerifyProof(mutationProof, iotago.Identifier(acceptedTransactionIDs.Root()), commitment.RootsID()) {
				w.LogError("failed to verify mutations proof", "commitment", commitment.LogName(), "transactionIDs", transactionIDs, "proof", mutationProof, "fromPeer", from)

				return false
			}

			w.ticker.StopTicker(commitmentID)

			// If the engine is "dirty" we need to restore the state of the engine to the state of the chain commitment.
			// As we already decided to switch and sync to this chain we should make sure that processing the blocks from the commitment
			// leads to the verified commitment.
			if targetEngine.Notarization.AcceptedBlocksCount(commitmentID.Slot()) > 0 {
				w.LogError("ENGINE IS DIRTY")
			}

			// Once all blocks are booked we
			//   1. Mark all transactions as accepted
			//   2. Mark all blocks as accepted
			//   3. Force commitment of the slot
			totalBlocks := uint32(len(blockIDs))
			var bookedBlocks atomic.Uint32
			var notarizedBlocks atomic.Uint32

			w.LogError("TOTAL BLOCKS IN RESPONSE", "totalBlocks", totalBlocks)

			forceCommitmentFunc := func() {
				// 3. Force commitment of the slot
				producedCommitment, err := targetEngine.Notarization.ForceCommit(commitmentID.Slot())
				if err != nil {
					w.protocol.LogError("failed to force commitment", "commitmentID", commitmentID, "err", err)

					return
				}

				// 4. Verify that the produced commitment is the same as the initially requested one
				if producedCommitment.ID() != commitmentID {
					w.protocol.LogError("commitment does not match", "expectedCommitmentID", commitmentID, "producedCommitmentID", producedCommitment.ID())

					return
				}
			}

			if len(blockIDs) == 0 {
				forceCommitmentFunc()

				return true
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

			for _, blockID := range blockIDs {
				w.LogError("requesting block", "blockID", blockID)

				block, _ := targetEngine.BlockDAG.GetOrRequestBlock(blockID)
				if block == nil {
					w.protocol.LogError("failed to request block", "blockID", blockID)

					continue
				}

				// We need to make sure that we add all blocks as root blocks because we don't know which blocks are root blocks without
				// blocks from future slots. We're committing the current slot which then leads to the eviction of the blocks from the
				// block cache and thus if not root blocks no block in the next slot can become solid.
				targetEngine.EvictionState.AddRootBlock(block.ID(), block.SlotCommitmentID())

				block.Booked().OnUpdate(blockBookedFunc)
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

		transactionIDs, err := committedSlot.TransactionIDs()
		if err != nil {
			w.LogTrace("failed to get transaction ids for warp-sync request", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from, "err", err)

			return
		}

		w.SendResponse(commitment, blockIDs, roots, transactionIDs, from)
	})
}

func (w *WarpSyncProtocol) Shutdown() {
	w.ticker.Shutdown()
	w.workerPool.Shutdown().ShutdownComplete.Wait()
}
