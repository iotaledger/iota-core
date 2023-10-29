package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

type BlocksProtocol struct {
	protocol            *Protocol
	workerPool          *workerpool.WorkerPool
	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

	log.Logger
}

func NewBlocksProtocol(protocol *Protocol) *BlocksProtocol {
	b := &BlocksProtocol{
		Logger:              lo.Return1(protocol.Logger.NewChildLogger("Blocks")),
		protocol:            protocol,
		workerPool:          protocol.Workers.CreatePool("Blocks"),
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
	}

	protocol.Constructed.OnTrigger(func() {
		protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.isDirectlyAboveLatestVerifiedCommitment.OnUpdate(func(_, isDirectlyAboveLatestVerifiedCommitment bool) {
				if !isDirectlyAboveLatestVerifiedCommitment {
					return
				}

				for _, droppedBlock := range b.droppedBlocksBuffer.GetValues(commitment.ID()) {
					b.ProcessResponse(droppedBlock.A, droppedBlock.B)
				}
			})
		})

		protocol.ChainManager.Chains.OnUpdate(func(mutations ds.SetMutations[*Chain]) {
			mutations.AddedElements().Range(func(chain *Chain) {
				chain.Engine.OnUpdate(func(_, engine *engine.Engine) {
					unsubscribe := engine.Events.BlockRequester.Tick.Hook(b.SendRequest).Unhook

					engine.Shutdown.OnTrigger(unsubscribe)
				})
			})
		})

		protocol.MainChain.Get().Engine.OnUpdateWithContext(func(_, engine *engine.Engine, unsubscribeOnEngineChange func(subscriptionFactory func() (unsubscribe func()))) {
			if engine != nil {
				unsubscribeOnEngineChange(func() (unsubscribe func()) {
					return lo.Batch(
						engine.Events.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) { b.SendResponse(block.ModelBlock()) }).Unhook,
						engine.Events.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) { b.SendResponse(block.ModelBlock()) }).Unhook,
					)
				})
			}
		})
	})

	return b
}

func (b *BlocksProtocol) SendRequest(blockID iotago.BlockID) {
	b.workerPool.Submit(func() {
		b.protocol.Network.RequestBlock(blockID)

		b.protocol.LogTrace("request", "blockID", blockID)
	})
}

func (b *BlocksProtocol) SendResponse(block *model.Block) {
	b.workerPool.Submit(func() {
		b.protocol.Network.SendBlock(block)

		b.protocol.LogTrace("sent block", "blockID", block.ID())
	})
}

func (b *BlocksProtocol) ProcessResponse(block *model.Block, from peer.ID) {
	b.workerPool.Submit(func() {
		commitmentRequest := b.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if commitmentRequest.WasRejected() {
			b.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID(), "err", commitmentRequest.Err())

			return
		}

		commitment := commitmentRequest.Result()
		if commitment == nil {
			if !b.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, from)) {
				b.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			} else {
				b.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		if !commitment.Chain.Get().DispatchBlock(block, from) {
			if !b.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, from)) {
				b.LogError("afailed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			} else {
				b.LogTrace("adropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		b.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName())
	})
}

func (b *BlocksProtocol) ProcessRequest(blockID iotago.BlockID, from peer.ID) {
	b.workerPool.Submit(func() {
		block, exists := b.protocol.MainEngine.Get().Block(blockID)
		if !exists {
			b.LogTrace("requested block not found", "blockID", blockID)

			return
		}

		b.protocol.Network.SendBlock(block, from)

		b.LogTrace("processed block request", "blockID", blockID)
	})
}

func (b *BlocksProtocol) Shutdown() {
	b.workerPool.Shutdown().ShutdownComplete.Wait()
}
