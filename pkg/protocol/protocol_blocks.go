package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

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
		protocol.Chains.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.ReplayDroppedBlocks.OnUpdate(func(_ bool, replayBlocks bool) {
				if replayBlocks {
					for _, droppedBlock := range b.droppedBlocksBuffer.GetValues(commitment.ID()) {
						b.LogTrace("replaying dropped block", "commitmentID", commitment.ID(), "blockID", droppedBlock.A.ID())

						b.ProcessResponse(droppedBlock.A, droppedBlock.B)
					}
				}
			})
		})

		protocol.Chains.WithElements(func(chain *Chain) func() {
			return chain.Engine.OnUpdate(func(_ *engine.Engine, engine *engine.Engine) {
				unsubscribe := engine.Events.BlockRequester.Tick.Hook(b.SendRequest).Unhook

				engine.Shutdown.OnTrigger(unsubscribe)
			})
		})

		protocol.Chains.Main.Get().Engine.OnUpdateWithContext(func(_ *engine.Engine, engine *engine.Engine, unsubscribeOnEngineChange func(subscriptionFactory func() (unsubscribe func()))) {
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

		b.LogTrace("request", "blockID", blockID)
	})
}

func (b *BlocksProtocol) SendResponse(block *model.Block) {
	b.workerPool.Submit(func() {
		b.protocol.Network.SendBlock(block)

		b.LogTrace("sent", "blockID", block.ID())
	})
}

func (b *BlocksProtocol) ProcessResponse(block *model.Block, from peer.ID) {
	b.workerPool.Submit(func() {
		commitmentRequest := b.protocol.Chains.requestCommitment(block.ProtocolBlock().Header.SlotCommitmentID, true)
		if commitmentRequest.WasRejected() {
			b.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID(), "err", commitmentRequest.Err())

			return
		}

		commitment := commitmentRequest.Result()
		if commitment == nil {
			if !b.droppedBlocksBuffer.Add(block.ProtocolBlock().Header.SlotCommitmentID, types.NewTuple(block, from)) {
				b.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			} else {
				b.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		if !commitment.Chain.Get().DispatchBlock(block, from) {
			if !b.droppedBlocksBuffer.Add(block.ProtocolBlock().Header.SlotCommitmentID, types.NewTuple(block, from)) {
				b.LogError("afailed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			} else {
				b.LogTrace("adropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		b.LogTrace("processed received block", "blockID", block.ID(), "commitment", commitment.LogName())
	})
}

func (b *BlocksProtocol) ProcessRequest(blockID iotago.BlockID, from peer.ID) {
	b.workerPool.Submit(func() {
		block, exists := b.protocol.Engines.Main.Get().Block(blockID)

		// TODO: CHECK why blocks can be nil here:
		//panic: runtime error: invalid memory address or nil pointer dereference
		//[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x10c2ac3]
		//
		//	goroutine 302684 [running]:
		//	github.com/iotaledger/iota-core/pkg/model.(*Block).Data(...)
		//	/actions-runner/_work/iota-core/iota-core/pkg/model/block.go:85
		//	github.com/iotaledger/iota-core/pkg/network/protocols/core.(*Protocol).SendBlock(0xc0045f0420, 0x0, {0xc0045a7c70, 0x1, 0x1})
		//	/actions-runner/_work/iota-core/iota-core/pkg/network/protocols/core/protocol.go:57 +0x43
		//	github.com/iotaledger/iota-core/pkg/protocol.(*BlocksProtocol).ProcessRequest-fm.(*BlocksProtocol).ProcessRequest.func1()
		//	/actions-runner/_work/iota-core/iota-core/pkg/protocol/protocol_blocks.go:128 +0x1ae
		//	github.com/iotaledger/hive.go/runtime/workerpool.(*Task).run(0xc0035f4060)
		//	/home/runner/go/pkg/mod/github.com/iotaledger/hive.go/runtime@v0.0.0-20231109195003-91f206338890/workerpool/task.go:48 +0xb6
		//	github.com/iotaledger/hive.go/runtime/workerpool.(*WorkerPool).workerReadLoop(0xc0021408c0)
		//	/home/runner/go/pkg/mod/github.com/iotaledger/hive.go/runtime@v0.0.0-20231109195003-91f206338890/workerpool/workerpool.go:190 +0x3b
		//	github.com/iotaledger/hive.go/runtime/workerpool.(*WorkerPool).worker(0xc0021408c0)
		//	/home/runner/go/pkg/mod/github.com/iotaledger/hive.go/runtime@v0.0.0-20231109195003-91f206338890/workerpool/workerpool.go:170 +0x69
		//	created by github.com/iotaledger/hive.go/runtime/workerpool.(*WorkerPool).startWorkers in goroutine 282432
		//	/home/runner/go/pkg/mod/github.com/iotaledger/hive.go/runtime@v0.0.0-20231109195003-91f206338890/workerpool/workerpool.go:162 +0x32
		//	FAIL	github.com/iotaledger/iota-core/pkg/tests	151.748s
		//	FAIL
		if !exists || block == nil {
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
