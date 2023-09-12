package protocol

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Gossip struct {
	protocol              *Protocol
	gossip                *event.Event1[*model.Block]
	attestationsRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentRequester   *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	blockRequestStarted   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequestStopped   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequested        *event.Event2[iotago.BlockID, *engine.Engine]
	commitmentVerifiers   *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]

	*logger.WrappedLogger
}

func NewGossip(protocol *Protocol) *Gossip {
	g := &Gossip{
		protocol:              protocol,
		gossip:                event.New1[*model.Block](),
		attestationsRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentRequester:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		blockRequestStarted:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequestStopped:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequested:        event.New2[iotago.BlockID, *engine.Engine](),
		commitmentVerifiers:   shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
		WrappedLogger:         logger.NewWrappedLogger(protocol.WrappedLogger.LoggerNamed("gossip")),
	}

	g.startAttestationsRequester()
	g.startBlockRequester()

	for _, gossipEvent := range []*event.Event1[*blocks.Block]{
		// TODO: REPLACE WITH REACTIVE VERSION
		protocol.Events.Engine.Scheduler.BlockScheduled,
		protocol.Events.Engine.Scheduler.BlockSkipped,
	} {
		gossipEvent.Hook(func(block *blocks.Block) { g.gossip.Trigger(block.ModelBlock()) })
	}

	return g
}

func (r *Gossip) IssueBlock(block *model.Block) error {
	r.protocol.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (r *Gossip) ProcessBlock(block *model.Block, src peer.ID) {
	commitmentRequest, err := r.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
	if err != nil {
		r.protocol.LogDebug(ierrors.Wrapf(err, "failed to process block %s from peer %s", block.ID(), src))

		return
	}

	if commitmentRequest.WasRejected() {
		r.protocol.LogDebug(ierrors.Wrapf(commitmentRequest.Err(), "failed to process block %s from peer %s", block.ID(), src))

		return
	}

	if !commitmentRequest.WasCompleted() {
		fmt.Println(r.protocol.MainEngineInstance().Name(), "WARNING3", block.ProtocolBlock().SlotCommitmentID, block.ProtocolBlock().SlotCommitmentID.Index())
		// TODO: QUEUE TO UNSOLID COMMITMENT BLOCKS
	} else {
		commitment := commitmentRequest.Result()

		if chain := commitment.Chain.Get(); chain != nil && chain.InSyncRange(block.ID().Index()) {
			if engine := commitment.Engine.Get(); engine != nil {
				engine.ProcessBlockFromPeer(block, src)
			}
		}
	}
}

func (r *Gossip) ProcessBlockRequest(blockID iotago.BlockID, src peer.ID) {
	block, exists := r.protocol.MainEngineInstance().Block(blockID)
	if !exists {
		r.LogDebug(ierrors.Errorf("requested block %s not found", blockID))

		return
	}

	r.protocol.SendBlock(block, src)
}

func (r *Gossip) ProcessCommitment(commitmentModel *model.Commitment, src peer.ID) {
	if _, err := r.protocol.PublishCommitment(commitmentModel); err != nil {
		r.LogDebug(ierrors.Wrapf(err, "failed to publish commitment %s from peer %s", commitmentModel.ID(), src))
	}
}

func (r *Gossip) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	if commitment, err := r.protocol.Commitment(commitmentID); err != nil {
		r.LogDebug(ierrors.Wrapf(err, "failed to process commitment request for commitment %s from peer %s", commitmentID, src))
	} else {
		r.protocol.SendSlotCommitment(commitment.Commitment, src)
	}
}

func (r *Gossip) ProcessAttestations(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) {
	commitment, err := r.protocol.PublishCommitment(commitmentModel)
	if err != nil {
		r.LogDebug(ierrors.Wrapf(err, "failed to publish commitment %s when processing attestations", commitmentModel.ID()))
		return
	}

	chain := commitment.Chain.Get()
	if chain == nil {
		r.LogDebug(ierrors.Errorf("failed to find chain for commitment %s when processing attestations", commitmentModel.ID()))
		return
	}

	commitmentVerifier, exists := r.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
	if !exists {
		r.LogDebug(ierrors.Errorf("failed to find commitment verifier for commitment %s when processing attestations", commitmentModel.ID()))
		return
	}

	blockIDs, actualCumulativeWeight, err := commitmentVerifier.verifyCommitment(commitmentModel, attestations, merkleProof)
	if err != nil {
		r.LogError(ierrors.Errorf("failed to verify commitment %s when processing attestations", commitmentModel.ID()))
		return
	}

	// TODO: publish blockIDs, actualCumulativeWeight to target commitment
	commitment.IsAttested.Set(true)

	fmt.Println("ATTESTATIONS", blockIDs, actualCumulativeWeight, source)
}

func (r *Gossip) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	mainEngine := r.protocol.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		r.LogDebug(ierrors.Errorf("requested commitment %s is not available, yet", commitmentID))

		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			r.LogDebug(ierrors.Wrapf(err, "failed to load commitment %s", commitmentID))
		} else {
			r.LogError(ierrors.Wrapf(err, "failed to load commitment %s", commitmentID))
		}

		return
	}

	if commitment.ID() != commitmentID {
		r.LogDebug(ierrors.Errorf("requested commitment is not from the main engine %s", commitmentID))

		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		r.LogError(ierrors.Wrapf(err, "failed to load attestations for commitment %s", commitmentID))

		return
	}

	rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Index())
	if err != nil {
		r.LogError(ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID))

		return
	}

	roots, err := rootsStorage.Load(commitmentID)
	if err != nil {
		r.LogError(ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID))

		return
	}

	r.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}

func (r *Gossip) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src peer.ID) {
	panic("implement me")
}

func (r *Gossip) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	panic("implement me")
}

func (r *Gossip) OnSendBlock(callback func(block *model.Block)) (unsubscribe func()) {
	return r.gossip.Hook(callback).Unhook
}

func (r *Gossip) OnBlockRequested(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return r.blockRequested.Hook(callback).Unhook
}

func (r *Gossip) OnBlockRequestStarted(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return r.blockRequestStarted.Hook(callback).Unhook
}

func (r *Gossip) OnBlockRequestStopped(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return r.blockRequestStopped.Hook(callback).Unhook
}

func (r *Gossip) OnCommitmentRequestStarted(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.commitmentRequester.Events.TickerStarted.Hook(callback).Unhook
}

func (r *Gossip) OnCommitmentRequestStopped(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.commitmentRequester.Events.TickerStopped.Hook(callback).Unhook
}

func (r *Gossip) OnCommitmentRequested(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.commitmentRequester.Events.Tick.Hook(callback).Unhook
}

func (r *Gossip) OnAttestationsRequestStarted(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.attestationsRequester.Events.TickerStarted.Hook(callback).Unhook
}

func (r *Gossip) OnAttestationsRequestStopped(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.attestationsRequester.Events.TickerStopped.Hook(callback).Unhook
}

func (r *Gossip) OnAttestationsRequested(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return r.attestationsRequester.Events.Tick.Hook(callback).Unhook
}

func (r *Gossip) startAttestationsRequester() {
	r.protocol.HookConstructed(func() {
		r.protocol.OnChainCreated(func(chain *Chain) {
			chain.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					r.commitmentVerifiers.GetOrCreate(chain.ForkingPoint.Get().ID(), func() *CommitmentVerifier {
						return NewCommitmentVerifier(chain.Engine.Get(), chain.ForkingPoint.Get().Parent.Get().Commitment)
					})
				} else {
					r.commitmentVerifiers.Delete(chain.ForkingPoint.Get().ID())
				}
			})
		})

		r.protocol.OnCommitmentCreated(func(commitment *Commitment) {
			commitment.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					r.attestationsRequester.StartTicker(commitment.ID())
				} else {
					r.attestationsRequester.StopTicker(commitment.ID())
				}
			})
		})
	})
}

func (r *Gossip) startBlockRequester() {
	startBlockRequester := func(engine *engine.Engine) {
		unsubscribe := lo.Batch(
			engine.Events.BlockRequester.Tick.Hook(func(id iotago.BlockID) {
				r.blockRequested.Trigger(id, engine)
			}).Unhook,

			engine.Events.BlockRequester.TickerStarted.Hook(func(id iotago.BlockID) {
				r.blockRequestStarted.Trigger(id, engine)
			}).Unhook,

			engine.Events.BlockRequester.TickerStopped.Hook(func(id iotago.BlockID) {
				r.blockRequestStopped.Trigger(id, engine)
			}).Unhook,
		)

		engine.HookShutdown(unsubscribe)
	}

	r.protocol.MainChain.Get().Engine.OnUpdate(func(_, engine *engine.Engine) {
		startBlockRequester(engine)
	})

	r.protocol.OnChainCreated(func(chain *Chain) {
		chain.Engine.OnUpdate(func(_, engine *engine.Engine) { startBlockRequester(engine) })
	})
}
