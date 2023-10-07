package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type GossipProtocol struct {
	protocol *Protocol

	inboundWorkers  *workerpool.WorkerPool
	outboundWorkers *workerpool.WorkerPool

	warpSyncRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
}

func NewGossipProtocol(protocol *Protocol) *GossipProtocol {
	g := &GossipProtocol{
		protocol: protocol,

		inboundWorkers:  protocol.Workers.CreatePool("Gossip.Inbound"),
		outboundWorkers: protocol.Workers.CreatePool("Gossip.Outbound"),

		warpSyncRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
	}

	protocol.HookConstructed(func() {
		g.startWarpSyncRequester()
	})

	return g
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
