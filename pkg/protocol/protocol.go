package protocol

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
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
						go p.ProcessReceivedBlock(droppedBlock.A, droppedBlock.B)
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

func (p *Protocol) ProcessReceivedBlock(block *model.Block, src peer.ID) {
	p.processTask("received block", func() (logLevel log.Level, err error) {
		logLevel = log.LevelTrace

		commitmentRequest := p.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if !commitmentRequest.WasCompleted() {
			if !p.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, src)) {
				return log.LevelError, ierrors.New("failed to add block to dropped blocks buffer")
			}

			return logLevel, ierrors.Errorf("referenced commitment %s unknown", block.ProtocolBlock().SlotCommitmentID)
		}

		if commitmentRequest.WasRejected() {
			return logLevel, commitmentRequest.Err()
		}

		if chain := commitmentRequest.Result().Chain.Get(); chain != nil {
			if chain.DispatchBlock(block, src) {
				p.LogError("block dropped", "blockID", block.ID(), "peer", src)
			}
		}

		return logLevel, nil
	}, "blockID", block.ID(), "peer", src)
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
