package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type GossipProtocol struct {
	protocol *Protocol

	inboundWorkers      *workerpool.WorkerPool
	outboundWorkers     *workerpool.WorkerPool
	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

	commitmentRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	warpSyncRequester   *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	blockRequested      *event.Event2[iotago.BlockID, *engine.Engine]
}

func NewGossipProtocol(protocol *Protocol) *GossipProtocol {
	g := &GossipProtocol{
		protocol: protocol,

		inboundWorkers:      protocol.Workers.CreatePool("Gossip.Inbound"),
		outboundWorkers:     protocol.Workers.CreatePool("Gossip.Outbound"),
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
		commitmentRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		warpSyncRequester:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		blockRequested:      event.New2[iotago.BlockID, *engine.Engine](),
	}

	protocol.HookConstructed(func() {
		g.startBlockRequester()
		g.startWarpSyncRequester()
		g.replayDroppedBlocks()
	})

	return g
}

func (g *GossipProtocol) ProcessBlock(block *model.Block, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		commitmentRequest := g.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if commitmentRequest.WasRejected() {
			g.protocol.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID(), "err", commitmentRequest.Err())

			return
		}

		commitment := commitmentRequest.Result()
		if commitment == nil || !commitment.Chain.Get().DispatchBlock(block, from) {
			if !g.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, from)) {
				g.protocol.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			} else {
				g.protocol.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		g.protocol.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName())
	})
}

func (g *GossipProtocol) ProcessBlockRequest(blockID iotago.BlockID, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		block, exists := g.protocol.MainEngineInstance().Block(blockID)
		if !exists {
			g.protocol.LogTrace("requested block not found", "blockID", blockID)

			return
		}

		g.protocol.Network.SendBlock(block, from)

		g.protocol.LogTrace("processed block request", "blockID", blockID)
	})
}

func (g *GossipProtocol) ProcessCommitment(commitmentModel *model.Commitment, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		if commitment, published, err := g.protocol.PublishCommitment(commitmentModel); err != nil {
			g.protocol.LogError("failed to process commitment", "fromPeer", from, "err", err)
		} else if published {
			g.protocol.LogDebug("processed received commitment", "commitment", commitment.LogName())
		}
	})
}

func (g *GossipProtocol) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		commitment, err := g.protocol.Commitment(commitmentID)
		if err != nil {
			logLevel := lo.Cond(ierrors.Is(err, ErrorCommitmentNotFound), log.LevelTrace, log.LevelError)

			g.protocol.Log("failed to load commitment for commitment request", logLevel, "commitmentID", commitmentID, "fromPeer", from, "error", err)

			return
		}

		g.protocol.Network.SendSlotCommitment(commitment.Commitment, from)

		g.protocol.LogTrace("processed commitment request", "commitment", commitment.LogName(), "fromPeer", from)
	})
}

func (g *GossipProtocol) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	g.inboundWorkers.Submit(func() {
		commitment, err := g.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				g.protocol.LogError("failed to load commitment for warp-sync response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				g.protocol.LogTrace("failed to load commitment for warp-sync response", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		targetEngine := commitment.Engine.Get()
		if targetEngine == nil {
			g.protocol.LogDebug("failed to get target engine for warp-sync response", "commitment", commitment.LogName())

			return
		}

		commitment.RequestedBlocksReceived.Compute(func(requestedBlocksReceived bool) bool {
			if requestedBlocksReceived || !commitment.RequestBlocks.Get() {
				g.protocol.LogTrace("warp-sync response for already synced commitment received", "commitment", commitment.LogName(), "fromPeer", from)

				return requestedBlocksReceived
			}

			acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
			for _, blockID := range blockIDs {
				_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
			}

			if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), commitment.RootsID()) {
				g.protocol.LogError("failed to verify merkle proof in warp-sync response", "commitment", commitment.LogName(), "blockIDs", blockIDs, "proof", proof, "fromPeer", from)

				return false
			}

			g.warpSyncRequester.StopTicker(commitmentID)

			for _, blockID := range blockIDs {
				targetEngine.BlockDAG.GetOrRequestBlock(blockID)
			}

			g.protocol.LogDebug("processed warp-sync response", "commitment", commitment.LogName())

			return true
		})
	})
}

func (g *GossipProtocol) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		commitment, err := g.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				g.protocol.LogError("failed to load commitment for warp-sync request", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			} else {
				g.protocol.LogTrace("failed to load commitment for warp-sync request", "commitmentID", commitmentID, "fromPeer", from, "err", err)
			}

			return
		}

		chain := commitment.Chain.Get()
		if chain == nil {
			g.protocol.LogTrace("warp-sync request for unsolid commitment", "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		engineInstance := commitment.Engine.Get()
		if engineInstance == nil {
			g.protocol.LogTrace("warp-sync request for chain without engine", "chain", chain.LogName(), "fromPeer", from)

			return
		}

		committedSlot, err := engineInstance.CommittedSlot(commitmentID)
		if err != nil {
			g.protocol.LogTrace("warp-sync request for uncommitted slot", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from)

			return
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			g.protocol.LogTrace("failed to get block ids for warp-sync request", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from, "err", err)

			return
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			g.protocol.LogTrace("failed to get roots for warp-sync request", "chain", chain.LogName(), "commitment", commitment.LogName(), "fromPeer", from, "err", err)

			return
		}

		g.protocol.Network.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), from)

		g.protocol.LogTrace("processed warp-sync request", "commitment", commitment.LogName(), "fromPeer", from)
	})
}

func (g *GossipProtocol) SendBlock(block *model.Block) {
	g.outboundWorkers.Submit(func() {
		g.protocol.Network.SendBlock(block)

		g.protocol.LogTrace("sent block", "blockID", block.ID())
	})
}

func (g *GossipProtocol) SendBlockRequest(blockID iotago.BlockID, engine *engine.Engine) {
	g.outboundWorkers.Submit(func() {
		g.protocol.Network.RequestBlock(blockID)

		g.protocol.LogTrace("sent block request", "engine", engine.Name(), "blockID", blockID)
	})
}

func (g *GossipProtocol) SendCommitmentRequest(commitmentID iotago.CommitmentID) {
	g.outboundWorkers.Submit(func() {
		g.protocol.Network.RequestSlotCommitment(commitmentID)

		g.protocol.LogDebug("sent commitment request", "commitmentID", commitmentID)
	})
}

func (g *GossipProtocol) SendWarpSyncRequest(id iotago.CommitmentID) {
	g.outboundWorkers.Submit(func() {
		if commitment, err := g.protocol.Commitment(id, false); err == nil {
			g.protocol.Network.SendWarpSyncRequest(id)

			g.protocol.LogDebug("sent warp-sync request", "commitment", commitment.LogName())
		}
	})
}

func (g *GossipProtocol) startWarpSyncRequester() {
	g.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.RequestBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
			if warpSyncBlocks {
				g.warpSyncRequester.StartTicker(commitment.ID())
			} else {
				g.warpSyncRequester.StopTicker(commitment.ID())
			}
		})
	})
}

func (g *GossipProtocol) startBlockRequester() {
	g.protocol.ChainManager.Chains.OnUpdate(func(mutations ds.SetMutations[*Chain]) {
		mutations.AddedElements().Range(func(chain *Chain) {
			chain.Engine.OnUpdate(func(_, engine *engine.Engine) {
				unsubscribe := lo.Batch(
					engine.Events.BlockRequester.Tick.Hook(func(id iotago.BlockID) {
						g.blockRequested.Trigger(id, engine)
					}).Unhook,
				)

				engine.HookShutdown(unsubscribe)
			})
		})
	})
}

func (g *GossipProtocol) replayDroppedBlocks() {
	g.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.InSyncRange.OnUpdate(func(_, inSyncRange bool) {
			if inSyncRange {
				for _, droppedBlock := range g.droppedBlocksBuffer.GetValues(commitment.ID()) {
					g.ProcessBlock(droppedBlock.A, droppedBlock.B)
				}
			}
		})
	})
}
