package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type GossipProtocol struct {
	*Protocol

	inboundWorkers      *workerpool.WorkerPool
	outboundWorkers     *workerpool.WorkerPool
	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]
}

func NewGossipProtocol(protocol *Protocol) *GossipProtocol {
	g := &GossipProtocol{
		Protocol:            protocol,
		inboundWorkers:      protocol.Workers.CreatePool("Gossip.Inbound"),
		outboundWorkers:     protocol.Workers.CreatePool("Gossip.Outbound"),
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
	}

	g.HookConstructed(func() {
		g.replayDroppedBlocks()
	})

	return g
}

func (g *GossipProtocol) ProcessBlock(block *model.Block, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		commitmentRequest := g.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if commitmentRequest.WasRejected() {
			g.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID(), "err", commitmentRequest.Err())

			return
		}

		commitment := commitmentRequest.Result()
		if commitment == nil || !commitment.Chain.Get().DispatchBlock(block, from) {
			if !g.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, from)) {
				g.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			} else {
				g.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		g.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName())
	})
}

func (g *GossipProtocol) ProcessBlockRequest(blockID iotago.BlockID, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		block, exists := g.MainEngineInstance().Block(blockID)
		if !exists {
			g.LogTrace("requested block not found", "blockID", blockID)

			return
		}

		g.SendBlock(block, from)

		g.LogTrace("processed block request", "blockID", blockID)
	})
}

func (g *GossipProtocol) ProcessCommitment(commitmentModel *model.Commitment, from peer.ID) {
	g.inboundWorkers.Submit(func() {
		if commitment, published, err := g.PublishCommitment(commitmentModel); err != nil {
			g.LogError("failed to process commitment", "fromPeer", from, "err", err)
		} else if published {
			g.LogDebug("processed received commitment", "commitment", commitment.LogName())
		}
	})
}

func (g *GossipProtocol) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, from peer.ID) {
	commitment, err := g.Commitment(commitmentID)
	if err != nil {
		logLevel := lo.Cond(ierrors.Is(err, ErrorCommitmentNotFound), log.LevelTrace, log.LevelError)

		g.Log("failed to load commitment for commitment request", logLevel, "commitmentID", commitmentID, "fromPeer", from, "error", err)

		return
	}

	g.Network.SendSlotCommitment(commitment.Commitment, from)

	g.LogTrace("processed commitment request", "commitment", commitment.LogName(), "fromPeer", from)
}

func (g *GossipProtocol) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], from peer.ID) {
	g.inboundWorkers.Submit(func() {
		chainCommitment, err := g.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				g.LogError("failed to load commitment for warp-sync response", "commitmentID", commitmentID, "err", err)
			} else {
				g.LogTrace("failed to load commitment for warp-sync response", "commitmentID", commitmentID, "err", err)
			}

			return
		}

		targetEngine := chainCommitment.Engine.Get()
		if targetEngine == nil {
			g.LogDebug("failed to get target engine for warp-sync response", "commitment", chainCommitment.LogName())

			return
		}

		chainCommitment.RequestedBlocksReceived.Compute(func(requestedBlocksReceived bool) bool {
			if requestedBlocksReceived || !chainCommitment.RequestBlocks.Get() {
				g.LogTrace("warp-sync response for already synced commitment received", "commitment", chainCommitment.LogName())

				return requestedBlocksReceived
			}

			acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
			for _, blockID := range blockIDs {
				_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
			}

			if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.RootsID()) {
				g.LogError("failed to verify merkle proof in warp-sync response", "commitment", chainCommitment.LogName(), "blockIDs", blockIDs, "proof", proof)

				return false
			}

			g.warpSyncRequester.StopTicker(commitmentID)

			for _, blockID := range blockIDs {
				targetEngine.BlockDAG.GetOrRequestBlock(blockID)
			}

			g.LogDebug("processed warp-sync response", "commitment", chainCommitment.LogName())

			return true
		})
	})
}

func (g *GossipProtocol) SendBlock(block *model.Block, to ...peer.ID) {
	g.outboundWorkers.Submit(func() {
		g.Network.SendBlock(block, to...)

		g.LogTrace("sent block", "blockID", block.ID(), "toPeers", to)
	})
}

func (g *GossipProtocol) SendAttestationsRequest(commitmentID iotago.CommitmentID) {
	g.outboundWorkers.Submit(func() {
		if commitment, err := g.Commitment(commitmentID, false); err == nil {
			g.Network.RequestAttestations(commitmentID)

			g.LogDebug("sent attestations request", "commitment", commitment.LogName())
		} else {
			g.LogError("failed to load commitment for attestations request", "commitmentID", commitmentID, "err", err)
		}
	})
}

func (g *GossipProtocol) SendWarpSyncRequest(id iotago.CommitmentID) {
	g.outboundWorkers.Submit(func() {
		if commitment, err := g.Commitment(id, false); err == nil {
			g.Network.SendWarpSyncRequest(id)

			g.LogDebug("sent warp-sync request", "commitment", commitment.LogName())
		}
	})
}

func (g *GossipProtocol) SendBlockRequest(blockID iotago.BlockID, engine *engine.Engine) {
	g.outboundWorkers.Submit(func() {
		g.Network.RequestBlock(blockID)

		g.LogTrace("sent block request", "engine", engine.Name(), "blockID", blockID)
	})
}

func (g *GossipProtocol) replayDroppedBlocks() {
	g.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.InSyncRange.OnUpdate(func(_, inSyncRange bool) {
			if inSyncRange {
				for _, droppedBlock := range g.droppedBlocksBuffer.GetValues(commitment.ID()) {
					g.ProcessBlock(droppedBlock.A, droppedBlock.B)
				}
			}
		})
	})
}
