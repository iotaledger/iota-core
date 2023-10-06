package protocol

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Protocol struct {
	Events              *Events
	Workers             *workerpool.Group
	error               *event.Event1[error]
	options             *Options
	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

	*NetworkManager
	*ChainManager
	*EngineManager
	log.Logger
	module.Module
}

func New(logger log.Logger, workers *workerpool.Group, dispatcher network.Endpoint, opts ...options.Option[Protocol]) *Protocol {
	return options.Apply(&Protocol{
		Events:              NewEvents(),
		Workers:             workers,
		Logger:              logger,
		error:               event.New1[error](),
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
		options:             newOptions(),
	}, opts, func(p *Protocol) {
		p.ChainManager = newChainManager(p)
		p.EngineManager = NewEngineManager(p)
		p.NetworkManager = newNetwork(p, dispatcher)

		p.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.InSyncRange.OnUpdate(func(_, inSyncRange bool) {
				if inSyncRange {
					for _, droppedBlock := range p.droppedBlocksBuffer.GetValues(commitment.ID()) {
						// TODO: replace with workerpool
						go p.ProcessBlock(droppedBlock.A, droppedBlock.B)
					}
				}
			})
		})
	}, (*Protocol).TriggerConstructed)
}

func (p *Protocol) Run(ctx context.Context) error {
	p.MainChain.OnUpdateOnce(func(_, mainChain *Chain) {
		mainChain.Engine.OnUpdateOnce(func(_, _ *engine.Engine) {
			p.TriggerInitialized()
		})
	})

	<-ctx.Done()

	p.TriggerShutdown()
	p.TriggerStopped()

	p.Workers.Shutdown()

	return ctx.Err()
}

func (p *Protocol) ProcessBlock(block *model.Block, peer peer.ID) {
	commitmentRequest := p.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
	if commitmentRequest.WasRejected() {
		p.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID(), "fromPeer", peer, "err", commitmentRequest.Err())
		return
	}

	commitment := commitmentRequest.Result()
	if commitment == nil || !commitment.Chain.Get().DispatchBlock(block, peer) {
		if !p.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, peer)) {
			p.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "blockID", block.ID(), "commitmentID", block.ProtocolBlock().SlotCommitmentID, "fromPeer", peer)
		} else {
			p.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "blockID", block.ID(), "commitmentID", block.ProtocolBlock().SlotCommitmentID, "fromPeer", peer)
		}
		return
	}

	p.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName(), "fromPeer", peer)
}

func (p *Protocol) ProcessBlockRequest(blockID iotago.BlockID, peer peer.ID) {
	block, exists := p.MainEngineInstance().Block(blockID)
	if !exists {
		p.LogTrace("requested block not found", "blockID", blockID, "fromPeer", peer)
		return
	}

	p.SendBlock(block, peer)

	p.LogTrace("processed block request", "blockID", blockID, "fromPeer", peer)
}

func (p *Protocol) SendBlock(block *model.Block, to ...peer.ID) {
	p.Network.SendBlock(block, to...)

	p.LogTrace("sent block", "blockID", block.ID(), "toPeers", to)
}

// APIForVersion returns the API for the given version.
func (p *Protocol) APIForVersion(version iotago.Version) (api iotago.API, err error) {
	return p.MainEngineInstance().APIForVersion(version)
}

// APIForSlot returns the API for the given slot.
func (p *Protocol) APIForSlot(slot iotago.SlotIndex) iotago.API {
	return p.MainEngineInstance().APIForSlot(slot)
}

func (p *Protocol) APIForEpoch(epoch iotago.EpochIndex) iotago.API {
	return p.MainEngineInstance().APIForEpoch(epoch)
}

func (p *Protocol) CurrentAPI() iotago.API {
	return p.MainEngineInstance().CurrentAPI()
}

func (p *Protocol) LatestAPI() iotago.API {
	return p.MainEngineInstance().LatestAPI()
}

func (p *Protocol) OnError(callback func(error)) (unsubscribe func()) {
	return p.error.Hook(callback).Unhook
}
