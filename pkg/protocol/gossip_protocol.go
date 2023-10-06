package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type GossipProtocol struct {
	*Protocol

	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]
}

func NewGossipProtocol(protocol *Protocol) *GossipProtocol {
	g := &GossipProtocol{
		Protocol:            protocol,
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
	}

	g.HookConstructed(func() {
		g.initDroppedBlocksReplay()
	})

	return g
}

func (g *GossipProtocol) initDroppedBlocksReplay() {
	g.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.InSyncRange.OnUpdate(func(_, inSyncRange bool) {
			if inSyncRange {
				for _, droppedBlock := range g.droppedBlocksBuffer.GetValues(commitment.ID()) {
					// TODO: replace with workerpool
					go g.ProcessBlock(droppedBlock.A, droppedBlock.B)
				}
			}
		})
	})
}

func (g *GossipProtocol) ProcessBlock(block *model.Block, from peer.ID) {
	commitmentRequest := g.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
	if commitmentRequest.WasRejected() {
		g.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID(), "fromPeer", from, "err", commitmentRequest.Err())
		return
	}

	commitment := commitmentRequest.Result()
	if commitment == nil || !commitment.Chain.Get().DispatchBlock(block, from) {
		if !g.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, from)) {
			g.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "blockID", block.ID(), "commitmentID", block.ProtocolBlock().SlotCommitmentID, "fromPeer", from)
		} else {
			g.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "blockID", block.ID(), "commitmentID", block.ProtocolBlock().SlotCommitmentID, "fromPeer", from)
		}
		return
	}

	g.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName(), "fromPeer", from)
}

func (g *GossipProtocol) ProcessBlockRequest(blockID iotago.BlockID, from peer.ID) {
	block, exists := g.MainEngineInstance().Block(blockID)
	if !exists {
		g.LogTrace("requested block not found", "blockID", blockID, "fromPeer", from)
		return
	}

	g.SendBlock(block, from)

	g.LogTrace("processed block request", "blockID", blockID, "fromPeer", from)
}

func (g *GossipProtocol) SendBlock(block *model.Block, to ...peer.ID) {
	g.Network.SendBlock(block, to...)

	g.LogTrace("sent block", "blockID", block.ID(), "toPeers", to)
}
