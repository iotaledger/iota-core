package protocol

import (
	"sync/atomic"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// WarpSync is a subcomponent of the protocol that is responsible for handling warp sync requests and responses.
type WarpSync struct {
	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// workerPool contains the worker pool that is used to process warp sync requests and responses asynchronously.
	workerPool *workerpool.WorkerPool

	// ticker contains the ticker that is used to send warp sync requests.
	ticker *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// newWarpSync creates a new warp sync protocol instance for the given protocol.
func newWarpSync(protocol *Protocol) *WarpSync {
	c := &WarpSync{
		Logger:     protocol.NewChildLogger("WarpSync"),
		protocol:   protocol,
		workerPool: protocol.Workers.CreatePool("WarpSync", workerpool.WithWorkerCount(1)),
		ticker:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](protocol.Options.WarpSyncRequesterOptions...),
	}

	// TODO: revert this debug hack
	//c.ticker.Events.Tick.Hook(c.SendRequest)

	protocol.ConstructedEvent().OnTrigger(func() {
		protocol.Chains.WithInitializedEngines(func(chain *Chain, engine *engine.Engine) (shutdown func()) {
			return chain.WarpSyncMode.OnUpdate(func(_ bool, warpSyncModeEnabled bool) {
				if warpSyncModeEnabled {
					engine.Workers.WaitChildren()
					engine.Reset()
				}
			})
		})

		protocol.Commitments.WithElements(func(commitment *Commitment) (shutdown func()) {
			return commitment.WarpSyncBlocks.OnUpdate(func(_ bool, warpSyncBlocks bool) {
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

// SendRequest sends a warp sync request for the given commitment ID to all peers.
func (w *WarpSync) SendRequest(commitmentID iotago.CommitmentID) {
	w.workerPool.Submit(func() {
		w.protocol.Network.SendWarpSyncRequest(commitmentID)

		w.LogDebug("request", "commitmentID", commitmentID)
	})
}

// SendResponse sends a warp sync response for the given commitment ID to the given peer.
func (w *WarpSync) SendResponse(commitment *Commitment, blockIDsBySlotCommitment map[iotago.CommitmentID]iotago.BlockIDs, roots *iotago.Roots, transactionIDs iotago.TransactionIDs, to peer.ID) {
	w.workerPool.Submit(func() {
		w.protocol.Network.SendWarpSyncResponse(commitment.ID(), blockIDsBySlotCommitment, roots.TangleProof(), transactionIDs, roots.MutationProof(), to)

		w.LogTrace("sent response", "commitment", commitment.LogName(), "toPeer", to)
	})
}

// ProcessResponse processes the given warp sync response.
func (w *WarpSync) ProcessResponse(commitmentID iotago.CommitmentID, blockIDsBySlotCommitment map[iotago.CommitmentID]iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationProof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	w.workerPool.Submit(func() {
		commitment, err := w.protocol.Commitments.Get(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				w.LogError("failed to load commitment for response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				w.LogTrace("failed to load commitment for response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		chain := commitment.Chain.Get()
		if chain == nil {
			w.LogTrace("failed to get chain for response", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		if !chain.WarpSyncMode.Get() {
			w.LogTrace("response for chain without warp-sync", "chain", chain.LogName(), "fromPeer", from)

			return
		}

		targetEngine := commitment.TargetEngine()
		if targetEngine == nil {
			w.LogDebug("failed to get target engine for response", "commitment", commitment.LogName())

			return
		}

		commitment.BlocksToWarpSync.Compute(func(blocksToWarpSync ds.Set[iotago.BlockID]) ds.Set[iotago.BlockID] {
			if blocksToWarpSync != nil || !commitment.WarpSyncBlocks.Get() {
				w.LogTrace("response for already synced commitment", "commitment", commitment.LogName(), "fromPeer", from)

				return blocksToWarpSync
			}

			totalBlocks := uint32(0)
			acceptedBlocks := ads.NewSet[iotago.Identifier](mapdb.NewMapDB(), iotago.Identifier.Bytes, iotago.IdentifierFromBytes, iotago.BlockID.Bytes, iotago.BlockIDFromBytes)
			for _, blockIDs := range blockIDsBySlotCommitment {
				for _, blockID := range blockIDs {
					_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error

					totalBlocks++
				}
			}

			if !iotago.VerifyProof(proof, acceptedBlocks.Root(), commitment.RootsID()) {
				w.LogError("failed to verify blocks proof", "commitment", commitment.LogName(), "blockIDs", blockIDsBySlotCommitment, "proof", proof, "fromPeer", from)

				return blocksToWarpSync
			}

			acceptedTransactionIDs := ads.NewSet[iotago.Identifier](mapdb.NewMapDB(), iotago.Identifier.Bytes, iotago.IdentifierFromBytes, iotago.TransactionID.Bytes, iotago.TransactionIDFromBytes)
			for _, transactionID := range transactionIDs {
				_ = acceptedTransactionIDs.Add(transactionID) // a mapdb can never return an error
			}

			if !iotago.VerifyProof(mutationProof, acceptedTransactionIDs.Root(), commitment.RootsID()) {
				w.LogError("failed to verify mutations proof", "commitment", commitment.ID(), commitment.Commitment.Commitment().String(), "acceptedTransactionIDsRoot", acceptedTransactionIDs.Root(), "transactionIDs len()", len(transactionIDs), "proof", mutationProof, "fromPeer", from)

				return blocksToWarpSync
			}

			w.ticker.StopTicker(commitmentID)

			targetEngine.Workers.WaitChildren()

			if !chain.WarpSyncMode.Get() {
				w.LogTrace("response for chain without warp-sync", "chain", chain.LogName(), "fromPeer", from)

				return blocksToWarpSync
			}

			// Once all blocks are booked we
			//   1. Mark all transactions as accepted
			//   2. Mark all blocks as accepted
			//   3. Force commitment of the slot
			commitmentFunc := func() {
				if !chain.WarpSyncMode.Get() {
					return
				}

				// 0. Prepare data flow
				var (
					notarizedBlocksCount uint64
					allBlocksNotarized   = reactive.NewEvent()
				)

				// 1. Mark all transactions as accepted
				for _, transactionID := range transactionIDs {
					targetEngine.Ledger.SpendDAG().SetAccepted(transactionID)
				}

				// 2. Mark all blocks as accepted and wait for them to be notarized
				if totalBlocks == 0 {
					allBlocksNotarized.Trigger()
				} else {
					for _, blockIDs := range blockIDsBySlotCommitment {
						for _, blockID := range blockIDs {
							block, exists := targetEngine.BlockCache.Block(blockID)
							if !exists { // this should never happen as we just booked these blocks in this slot.
								continue
							}

							targetEngine.BlockGadget.SetAccepted(block)

							block.Notarized().OnTrigger(func() {
								if atomic.AddUint64(&notarizedBlocksCount, 1) == uint64(totalBlocks) {
									allBlocksNotarized.Trigger()
								}
							})
						}
					}
				}

				allBlocksNotarized.OnTrigger(func() {
					// This needs to happen in a separate worker since the trigger for block notarized while the lock in
					// the notarization is still held.
					w.workerPool.Submit(func() {
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
					})
				})
			}

			// Once all blocks are fully booked we can mark the commitment that is minCommittableAge older as this
			// commitment to be committable.
			commitment.IsSynced.OnUpdateOnce(func(_ bool, _ bool) {
				// update the flag in a worker since it can potentially cause a commit
				w.workerPool.Submit(func() {
					// we add +1 here to enable syncing of chains of empty commitments since we can assume that there is
					// at least 1 additional slot building on top of the synced commitment as it would have otherwise
					// not turned into a commitment in the first place.
					if committableCommitment, exists := chain.Commitment(commitmentID.Slot() - targetEngine.LatestAPI().ProtocolParameters().MinCommittableAge() + 1); exists {
						committableCommitment.IsCommittable.Set(true)
					}
				})
			})

			// force commit one by one and wait for the parent to be verified before we commit the next one
			commitment.Parent.WithNonEmptyValue(func(parent *Commitment) (teardown func()) {
				return parent.IsVerified.WithNonEmptyValue(func(_ bool) (teardown func()) {
					return commitment.IsCommittable.OnTrigger(commitmentFunc)
				})
			})

			if totalBlocks == 0 {
				// mark empty slots as committable and synced
				commitment.IsCommittable.Set(true)
				commitment.IsSynced.Set(true)

				return blocksToWarpSync
			}

			var bookedBlocks atomic.Uint32
			blocksToWarpSync = ds.NewSet[iotago.BlockID]()
			for _, blockIDs := range blockIDsBySlotCommitment {
				for _, blockID := range blockIDs {
					blocksToWarpSync.Add(blockID)

					block, _ := targetEngine.BlockDAG.GetOrRequestBlock(blockID)
					if block == nil {
						w.protocol.LogError("failed to request block", "blockID", blockID)

						continue
					}

					// We need to make sure that all blocks are fully booked and their weight propagated before we can
					// move the window forward. This is in order to ensure that confirmation and finalization is correctly propagated.
					block.WeightPropagated().OnUpdate(func(_ bool, _ bool) {
						if bookedBlocks.Add(1) != totalBlocks {
							return
						}

						commitment.IsSynced.Set(true)
					})
				}
			}

			w.LogDebug("received response", "commitment", commitment.LogName())

			return blocksToWarpSync
		})
	})
}

// ProcessRequest processes the given warp sync request.
func (w *WarpSync) ProcessRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	loggedWorkerPoolTask(w.workerPool, func() (err error) {
		commitmentAPI, err := w.protocol.Commitments.API(commitmentID)
		if err != nil {
			return ierrors.Wrap(err, "failed to load slot api")
		}

		blocks, blocksProof, transactionIDs, transactionIDsProof, err := commitmentAPI.Mutations()
		if err != nil {
			return ierrors.Wrap(err, "failed to get mutations")
		}

		w.protocol.Network.SendWarpSyncResponse(commitmentID, blocks, blocksProof, transactionIDs, transactionIDsProof, from)

		return nil
	}, w, "commitmentID", commitmentID, "fromPeer", from)
}

// Shutdown shuts down the warp sync protocol.
func (w *WarpSync) Shutdown() {
	w.ticker.Shutdown()
}
