package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// BlocksProtocol is a subcomponent of the protocol that is responsible for handling block requests and responses.
type BlocksProtocol struct {
	// protocol contains a reference to the Protocol instance that this component belongs to.
	protocol *Protocol

	// workerPool contains the worker pool that is used to process block requests and responses asynchronously.
	workerPool *workerpool.WorkerPool

	// droppedBlocksBuffer contains a buffer for dropped blocks that reference unsolid commitments that can be replayed
	// at a later point in time (to make tests more reliable as we have no continuous activity).
	droppedBlocksBuffer *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// newBlocksProtocol creates a new blocks protocol instance for the given protocol.
func newBlocksProtocol(protocol *Protocol) *BlocksProtocol {
	b := &BlocksProtocol{
		Logger:              lo.Return1(protocol.Logger.NewChildLogger("Blocks")),
		protocol:            protocol,
		workerPool:          protocol.Workers.CreatePool("Blocks"),
		droppedBlocksBuffer: buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
	}

	protocol.Constructed.OnTrigger(func() {
		protocol.Commitments.WithElements(func(commitment *Commitment) (shutdown func()) {
			return commitment.ReplayDroppedBlocks.OnUpdate(func(_ bool, replayBlocks bool) {
				if replayBlocks {
					for _, droppedBlock := range b.droppedBlocksBuffer.GetValues(commitment.ID()) {
						b.LogTrace("replaying dropped block", "commitmentID", commitment.ID(), "blockID", droppedBlock.A.ID())

						b.ProcessResponse(droppedBlock.A, droppedBlock.B)
					}
				}
			})
		})

		protocol.Chains.WithInitializedEngines(func(chain *Chain, engine *engine.Engine) (shutdown func()) {
			return lo.Batch(
				engine.Events.BlockRequester.Tick.Hook(b.SendRequest).Unhook,
				engine.Events.Scheduler.BlockScheduled.Hook(func(block *blocks.Block) { b.SendResponse(block.ModelBlock()) }).Unhook,
				engine.Events.Scheduler.BlockSkipped.Hook(func(block *blocks.Block) { b.SendResponse(block.ModelBlock()) }).Unhook,
			)
		})
	})

	return b
}

// SendRequest sends a request for the given block to all peers.
func (b *BlocksProtocol) SendRequest(blockID iotago.BlockID) {
	b.workerPool.Submit(func() {
		b.protocol.Network.RequestBlock(blockID)

		b.LogTrace("request", "blockID", blockID)
	})
}

// SendResponse sends the given block to all peers.
func (b *BlocksProtocol) SendResponse(block *model.Block) {
	b.workerPool.Submit(func() {
		b.protocol.Network.SendBlock(block)

		b.LogTrace("sent", "blockID", block.ID())
	})
}

// ProcessResponse processes the given block response.
func (b *BlocksProtocol) ProcessResponse(block *model.Block, from peer.ID) {
	b.workerPool.Submit(func() {
		// abort if the commitment belongs to an evicted slot
		commitmentMetadata, err := b.protocol.Commitments.Metadata(block.ProtocolBlock().Header.SlotCommitmentID, true)
		if err != nil && ierrors.Is(ErrorSlotEvicted, err) {
			b.LogError("dropped block referencing unsolidifiable commitment", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID(), "err", err)

			return
		}

		// add the block to the dropped blocks buffer if we could not dispatch it to the chain
		if commitmentMetadata == nil || !commitmentMetadata.Chain.Get().DispatchBlock(block, from) {
			if !b.droppedBlocksBuffer.Add(block.ProtocolBlock().Header.SlotCommitmentID, types.NewTuple(block, from)) {
				b.LogError("failed to add dropped block referencing unsolid commitment to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			} else {
				b.LogTrace("dropped block referencing unsolid commitment added to dropped blocks buffer", "commitmentID", block.ProtocolBlock().Header.SlotCommitmentID, "blockID", block.ID())
			}

			return
		}

		b.LogTrace("received block", "blockID", block.ID(), "commitment", commitmentMetadata.LogName())
	})
}

// ProcessRequest processes the given block request.
func (b *BlocksProtocol) ProcessRequest(blockID iotago.BlockID, from peer.ID) {
	b.workerPool.Submit(func() {
		block, exists := b.protocol.Engines.Main.Get().Block(blockID)
		if !exists {
			b.LogTrace("requested block not found", "blockID", blockID)

			return
		}

		b.protocol.Network.SendBlock(block, from)

		b.LogTrace("processed block request", "blockID", blockID)
	})
}

// Shutdown shuts down the blocks protocol and waits for all pending requests to be finished.
func (b *BlocksProtocol) Shutdown() {
	b.workerPool.Shutdown().ShutdownComplete.Wait()
}
