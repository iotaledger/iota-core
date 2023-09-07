package protocol

import (
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
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
	}

	g.startAttestationsRequester()
	g.startBlockRequester()

	for _, gossipEvent := range []*event.Event1[*blocks.Block]{
		protocol.MainEngineEvents.Scheduler.BlockScheduled,
		protocol.MainEngineEvents.Scheduler.BlockSkipped,
	} {
		gossipEvent.Hook(func(block *blocks.Block) { g.gossip.Trigger(block.ModelBlock()) })
	}

	return g
}

func (r *Gossip) IssueBlock(block *model.Block) error {
	r.protocol.MainEngine().ProcessBlockFromPeer(block, "self")

	return nil
}

func (r *Gossip) ProcessBlock(block *model.Block, src peer.ID) error {
	commitmentRequest, err := r.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
	if err != nil {
		return ierrors.Wrapf(err, "failed to process block %s from peer %s", block.ID(), src)
	} else if commitmentRequest.WasRejected() {
		return ierrors.Wrapf(commitmentRequest.Err(), "failed to process block %s from peer %s", block.ID(), src)
	} else if !commitmentRequest.WasCompleted() {
		fmt.Println("WARNING3", block.ProtocolBlock().SlotCommitmentID)
		// TODO: QUEUE TO UNSOLID COMMITMENT BLOCKS
	} else {
		commitment := commitmentRequest.Result()

		if chain := commitment.Chain(); chain != nil && chain.InSyncRange(block.ID().Index()) {
			if engine := commitment.Engine().Get(); engine != nil {
				engine.ProcessBlockFromPeer(block, src)
			}
		}
	}

	return nil
}

func (r *Gossip) ProcessBlockRequest(blockID iotago.BlockID, src peer.ID) error {
	block, exists := r.protocol.MainEngine().Block(blockID)
	if !exists {
		// TODO: CREATE SENTINAL ERRORS
		return ierrors.Errorf("requested block %s not found", blockID)
	}

	r.protocol.SendBlock(block, src)

	return nil
}

func (r *Gossip) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, src peer.ID) error {
	if commitment, err := r.protocol.Commitment(commitmentID); err == nil {
		r.protocol.SendSlotCommitment(commitment.CommitmentModel(), src)
	} else if !ierrors.Is(err, ErrorCommitmentNotFound) {
		return ierrors.Wrapf(err, "failed to process slot commitment request for commitment %s from peer %s", commitmentID, src)
	}

	return nil
}

func (r *Gossip) ProcessAttestationsResponse(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) (err error) {
	commitment, err := r.protocol.PublishCommitment(commitmentModel)
	if err != nil {
		return ierrors.Wrapf(err, "failed to publish commitment %s when processing attestations", commitmentModel.ID())
	}

	chain := commitment.Chain()
	if chain == nil {
		return ierrors.Errorf("failed to find chain for commitment %s when processing attestations", commitmentModel.ID())
	}

	commitmentVerifier, exists := r.commitmentVerifiers.Get(chain.Root().ID())
	if !exists {
		return ierrors.Errorf("failed to find commitment verifier for commitment %s when processing attestations", commitmentModel.ID())
	}

	blockIDs, actualCumulativeWeight, err := commitmentVerifier.verifyCommitment(commitmentModel, attestations, merkleProof)
	if err != nil {
		return ierrors.Errorf("failed to verify commitment %s when processing attestations", commitmentModel.ID())
	}

	// TODO: publish blockIDs, actualCumulativeWeight to target commitment
	commitment.Attested().Set(true)

	fmt.Println("ATTESTATIONS", blockIDs, actualCumulativeWeight, source)

	return nil
}

func (r *Gossip) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) error {
	mainEngine := r.protocol.MainEngine()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return ierrors.Errorf("main engine has produced the requested commitment %s, yet", commitmentID)
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		return ierrors.Wrapf(err, "failed to load commitment %s", commitmentID)
	}

	if commitment.ID() != commitmentID {
		return ierrors.Errorf("requested commitment is not from the main chain %s", commitmentID)
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		return ierrors.Wrapf(err, "failed to load attestations for commitment %s", commitmentID)
	}

	rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Index())
	if err != nil {
		return ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID)
	}

	roots, err := rootsStorage.Load(commitmentID)
	if err != nil {
		return ierrors.Wrapf(err, "failed to load roots for commitment %s", commitmentID)
	}

	r.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)

	return nil
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
			chain.RequestAttestations().OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					r.commitmentVerifiers.GetOrCreate(chain.Root().ID(), func() *CommitmentVerifier {
						return NewCommitmentVerifier(chain.EngineR().Get(), chain.Root().Parent().Get().CommitmentModel())
					})
				} else {
					r.commitmentVerifiers.Delete(chain.Root().ID())
				}
			})
		})

		r.protocol.OnCommitmentCreated(func(commitment *Commitment) {
			commitment.requestAttestations.OnUpdate(func(_, requestAttestations bool) {
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

	startBlockRequester(r.protocol.MainEngine())

	r.protocol.OnEngineCreated(startBlockRequester)
}
