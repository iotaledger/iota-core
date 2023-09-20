package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
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
	warpSyncRequester     *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	blockRequestStarted   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequestStopped   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequested        *event.Event2[iotago.BlockID, *engine.Engine]
	commitmentVerifiers   *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]

	log.Logger
}

func NewGossip(protocol *Protocol) *Gossip {
	g := &Gossip{
		protocol:              protocol,
		gossip:                event.New1[*model.Block](),
		attestationsRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentRequester:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		warpSyncRequester:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		blockRequestStarted:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequestStopped:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequested:        event.New2[iotago.BlockID, *engine.Engine](),
		commitmentVerifiers:   shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	g.startBlockRequester()
	g.startAttestationsRequester()
	g.startWarpSyncRequester()

	for _, gossipEvent := range []*event.Event1[*blocks.Block]{
		// TODO: REPLACE WITH REACTIVE VERSION
		protocol.Events.Engine.Scheduler.BlockScheduled,
		protocol.Events.Engine.Scheduler.BlockSkipped,
	} {
		gossipEvent.Hook(func(block *blocks.Block) { g.gossip.Trigger(block.ModelBlock()) })
	}

	g.Logger = func() log.Logger {
		logger, shutdownLogger := protocol.Logger.NewChildLogger("Gossip")

		protocol.HookShutdown(shutdownLogger)

		return logger
	}()

	return g
}

func (r *Gossip) IssueBlock(block *model.Block) error {
	r.protocol.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (r *Gossip) ProcessBlock(block *model.Block, src peer.ID) {
	commitmentRequest, err := r.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
	if err != nil {
		r.protocol.LogDebug("failed to process block", "blockID", block.ID(), "source", src, "err", err)

		return
	}

	if commitmentRequest.WasRejected() {
		r.protocol.LogDebug("failed to process block", "blockID", block.ID(), "source", src, "err", commitmentRequest.Err())

		return
	}

	if !commitmentRequest.WasCompleted() {
		//fmt.Println(r.protocol.MainEngineInstance().Name(), "WARNING3", block.ProtocolBlock().SlotCommitmentID, block.ProtocolBlock().SlotCommitmentID.Index())
		// TODO: QUEUE TO UNSOLID COMMITMENT BLOCKS
		r.protocol.LogTrace("failed to process block", "blockID", block.ID(), "peer", src, "err", ierrors.New("block still missing"))
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
	if block, exists := r.protocol.MainEngineInstance().Block(blockID); !exists {
		r.LogDebug("failed to load requested block", "blockID", blockID, "peer", src)
	} else {
		r.protocol.SendBlock(block, src)
	}
}

func (r *Gossip) ProcessCommitment(commitmentModel *model.Commitment, src peer.ID) {
	if _, err := r.protocol.PublishCommitment(commitmentModel); err != nil {
		r.LogDebug("failed to publish commitment", "commitmentID", commitmentModel.ID(), "peer", src)
	}
}

func (r *Gossip) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	if commitment, err := r.protocol.Commitment(commitmentID); err != nil {
		if !ierrors.Is(err, ErrorCommitmentNotFound) {
			r.LogDebug("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		} else {
			r.LogTrace("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		}
	} else {
		r.protocol.SendSlotCommitment(commitment.Commitment, src)
	}
}

func (r *Gossip) ProcessAttestations(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) {
	commitment, err := r.protocol.PublishCommitment(commitmentModel)
	if err != nil {
		r.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitmentModel.ID(), "peer", source, "error", err)
		return
	}

	chain := commitment.Chain.Get()
	if chain == nil {
		r.LogDebug("failed to find chain for commitment when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	commitmentVerifier, exists := r.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
	if !exists {
		r.LogDebug("failed to find commitment verifier for commitment %s when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	_, actualWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)
	if err != nil {
		r.LogError("failed to verify commitment when processing attestations", "commitmentID", commitmentModel.ID(), "error", err)
		return
	}

	commitment.AttestedWeight.Set(actualWeight)
	commitment.IsAttested.Set(true)
}

func (r *Gossip) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	mainEngine := r.protocol.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		r.LogDebug("requested commitment is not verified, yet", "commitmentID", commitmentID)

		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			r.LogDebug("failed to load commitment", "commitmentID", commitmentID)
		} else {
			r.LogError("failed to load commitment", "commitmentID", commitmentID)
			panic("here4")
		}

		return
	}

	if commitment.ID() != commitmentID {
		r.LogTrace("requested commitment does not belong to main engine", "requestedCommitmentID", commitmentID, "mainEngineCommitmentID", commitment.ID())
		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		r.LogError("failed to load attestations", "commitmentID", commitmentID, "error", err)
		panic("here")
		return
	}

	rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Index())
	if err != nil {
		r.LogError("failed to load roots", "commitmentID", commitmentID, "error", err)
		panic("here1")
		return
	}

	roots, err := rootsStorage.Load(commitmentID)
	if err != nil {
		r.LogError("failed to load roots", "commitmentID", commitmentID, "error", err)
		panic("here2")
		return
	}

	r.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}

func (r *Gossip) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src peer.ID) {
	if err, logLevel := func() (err error, logLevel log.Level) {
		chainCommitment, err := r.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				return ierrors.Wrapf(err, "unexpected error when loading commitment"), log.LevelError
			}

			return ierrors.Wrapf(err, "warp sync response for unknown commitment"), log.LevelDebug
		}

		if !chainCommitment.WarpSyncBlocks.Get() {
			return ierrors.New("warp sync not requested"), log.LevelDebug
		}

		targetEngine := chainCommitment.Engine.Get()
		if targetEngine == nil {
			return ierrors.New("failed to get engine"), log.LevelDebug
		}

		acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
		for _, blockID := range blockIDs {
			_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
		}

		if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.RootsID()) {
			return ierrors.New("failed to verify merkle proof"), log.LevelError
		}

		r.warpSyncRequester.StopTicker(commitmentID)

		for _, blockID := range blockIDs {
			targetEngine.BlockDAG.GetOrRequestBlock(blockID)
		}

		return
	}(); err != nil {
		r.Log("failed to process warp sync response", logLevel, "commitmentID", commitmentID, "peer", src, "error", err)
	} else {
		r.Log("successfully processed warp sync response", log.LevelDebug, "commitmentID", commitmentID, "peer", src)
	}
}

func (r *Gossip) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	if err, logLevel := func() (err error, logLevel log.Level) {
		committedSlot, err := r.protocol.MainEngineInstance().CommittedSlot(commitmentID)
		if err != nil {
			return ierrors.Wrap(err, "failed to get slot for commitment"), log.LevelDebug
		}

		commitment, err := committedSlot.Commitment()
		if err != nil {
			return ierrors.Wrap(err, "failed to get commitment from slot"), log.LevelDebug
		} else if commitment.ID() != commitmentID {
			return ierrors.New("commitment ID mismatch"), log.LevelError
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			return ierrors.Wrap(err, "failed to get block IDs from slot"), log.LevelDebug
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			return ierrors.Wrap(err, "failed to get roots from slot"), log.LevelDebug
		}

		r.protocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), src)

		return
	}(); err != nil {
		r.Log("failed to process warp sync request", logLevel, "commitmentID", commitmentID, "peer", src, "error", err)
	} else {
		r.Log("successfully processed warp sync request", log.LevelDebug, "commitmentID", commitmentID, "peer", src)
	}
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
		r.protocol.ChainCreated.Hook(func(chain *Chain) {
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

		r.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					if commitment.CumulativeWeight() == 0 {
						commitment.IsAttested.Set(true)
					} else {
						r.attestationsRequester.StartTicker(commitment.ID())
					}
				} else {
					r.attestationsRequester.StopTicker(commitment.ID())
				}
			})
		})
	})
}

func (r *Gossip) startWarpSyncRequester() {
	r.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.WarpSyncBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
			if warpSyncBlocks {
				r.warpSyncRequester.StartTicker(commitment.ID())
			} else {
				r.warpSyncRequester.StopTicker(commitment.ID())
			}
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

	r.protocol.ChainCreated.Hook(func(chain *Chain) {
		chain.Engine.OnUpdate(func(_, engine *engine.Engine) { startBlockRequester(engine) })
	})
}
