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
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Network struct {
	protocol *Protocol

	*core.Protocol

	gossip                *event.Event1[*model.Block]
	attestationsRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentRequester   *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	warpSyncRequester     *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	blockRequestStarted   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequestStopped   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequested        *event.Event2[iotago.BlockID, *engine.Engine]
	commitmentVerifiers   *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]

	log.Logger

	module.Module
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *Network {
	n := &Network{
		protocol:              protocol,
		Protocol:              core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
		gossip:                event.New1[*model.Block](),
		attestationsRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentRequester:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		warpSyncRequester:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		blockRequestStarted:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequestStopped:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequested:        event.New2[iotago.BlockID, *engine.Engine](),
		commitmentVerifiers:   shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
	}

	n.Logger = func() log.Logger {
		logger, shutdownLogger := protocol.Logger.NewChildLogger("Gossip")

		protocol.HookShutdown(shutdownLogger)

		return logger
	}()

	n.TriggerConstructed()

	n.startBlockRequester()
	n.startAttestationsRequester()
	n.startWarpSyncRequester()

	for _, gossipEvent := range []*event.Event1[*blocks.Block]{
		// TODO: REPLACE WITH REACTIVE VERSION
		protocol.Events.Engine.Scheduler.BlockScheduled,
		protocol.Events.Engine.Scheduler.BlockSkipped,
	} {
		gossipEvent.Hook(func(block *blocks.Block) { n.gossip.Trigger(block.ModelBlock()) })
	}

	var unsubscribeFromNetworkEvents func()

	protocol.HookInitialized(func() {
		n.OnError(func(err error, peer peer.ID) {
			n.LogError("network error", "peer", peer, "error", err)
		})

		unsubscribeFromNetworkEvents = lo.Batch(
			n.OnBlockReceived(n.ProcessReceivedBlock),
			n.OnBlockRequestReceived(n.ProcessBlockRequest),
			n.OnCommitmentReceived(n.ProcessCommitment),
			n.OnCommitmentRequestReceived(n.ProcessCommitmentRequest),
			n.OnAttestationsReceived(n.ProcessAttestations),
			n.OnAttestationsRequestReceived(n.ProcessAttestationsRequest),
			n.OnWarpSyncResponseReceived(n.ProcessWarpSyncResponse),
			n.OnWarpSyncRequestReceived(n.ProcessWarpSyncRequest),

			n.warpSyncRequester.Events.Tick.Hook(func(id iotago.CommitmentID) {
				n.LogDebug("request warp sync", "commitmentID", id)

				n.SendWarpSyncRequest(id)
			}).Unhook,
			n.OnSendBlock(func(block *model.Block) { n.SendBlock(block) }),
			n.OnBlockRequested(func(blockID iotago.BlockID, engine *engine.Engine) { n.RequestBlock(blockID) }),
			n.OnCommitmentRequested(func(id iotago.CommitmentID) {
				n.LogInfo("commitment requested", "commitmentID", id)

				n.RequestSlotCommitment(id)
			}),
			n.OnAttestationsRequested(func(commitmentID iotago.CommitmentID) { n.RequestAttestations(commitmentID) }),
		)

		n.TriggerInitialized()

		protocol.HookShutdown(func() {
			n.TriggerShutdown()

			unsubscribeFromNetworkEvents()

			n.Protocol.Shutdown()

			n.TriggerStopped()
		})
	})

	return n
}

func (n *Network) IssueBlock(block *model.Block) error {
	n.protocol.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (n *Network) ProcessReceivedBlock(block *model.Block, src peer.ID) {
	n.processTask("received block", func() (err error, logLevel log.Level) {
		commitmentRequest := n.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if !commitmentRequest.WasCompleted() {
			return ierrors.Errorf("UNSOLID COMMITMENT WITH %s", block.ProtocolBlock().SlotCommitmentID), log.LevelDebug
		}

		if commitmentRequest.WasRejected() {
			return commitmentRequest.Err(), log.LevelDebug
		}

		commitment := commitmentRequest.Result()
		chain := commitment.Chain.Get()

		if chain == nil {
			return ierrors.Errorf("UNSOLID CHAIN FOR %s", block.ProtocolBlock().SlotCommitmentID), log.LevelDebug
		}

		if !chain.InSyncRange(block.ID().Index()) {
			return ierrors.Errorf("NOT IN SYNC RANGE %s", block.ProtocolBlock().SlotCommitmentID), log.LevelDebug
		}

		engine := commitment.Engine.Get()
		if engine == nil {
			return ierrors.Errorf("ENGINE NOT KNOWN FOR %s", block.ProtocolBlock().SlotCommitmentID), log.LevelDebug
		}

		engine.ProcessBlockFromPeer(block, src)

		return nil, log.LevelTrace
	})
}

func (n *Network) ProcessBlockRequest(blockID iotago.BlockID, src peer.ID) {
	if block, exists := n.protocol.MainEngineInstance().Block(blockID); !exists {
		n.LogDebug("failed to load requested block", "blockID", blockID, "peer", src)
	} else {
		n.protocol.SendBlock(block, src)
	}
}

func (n *Network) ProcessCommitment(commitmentModel *model.Commitment, src peer.ID) {
	if _, err := n.protocol.PublishCommitment(commitmentModel); err != nil {
		n.LogDebug("failed to publish commitment", "commitmentID", commitmentModel.ID(), "peer", src)
	} else {
		n.LogDebug("successfully published commitment", "commitmentID", commitmentModel.ID(), "peer", src)
	}
}

func (n *Network) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.LogTrace("commitment request received", "commitmentID", commitmentID, "peer", src)

	if commitment, err := n.protocol.Commitment(commitmentID); err != nil {
		if !ierrors.Is(err, ErrorCommitmentNotFound) {
			n.LogDebug("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		} else {
			n.LogTrace("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		}
	} else {
		n.LogDebug("sending commitment", "commitmentID", commitmentID, "peer", src)

		n.protocol.SendSlotCommitment(commitment.Commitment, src)
	}
}

func (n *Network) ProcessAttestations(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) {
	commitment, err := n.protocol.PublishCommitment(commitmentModel)
	if err != nil {
		n.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitmentModel.ID(), "peer", source, "error", err)
		return
	}

	chain := commitment.Chain.Get()
	if chain == nil {
		n.LogDebug("failed to find chain for commitment when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	commitmentVerifier, exists := n.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
	if !exists {
		n.LogDebug("failed to find commitment verifier for commitment %s when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	_, actualWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)
	if err != nil {
		n.LogError("failed to verify commitment when processing attestations", "commitmentID", commitmentModel.ID(), "error", err)
		return
	}

	commitment.AttestedWeight.Set(actualWeight)
	commitment.IsAttested.Set(true)
}

func (n *Network) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	mainEngine := n.protocol.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		n.LogDebug("requested commitment is not verified, yet", "commitmentID", commitmentID)

		return
	}

	commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index())
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			n.LogDebug("failed to load commitment", "commitmentID", commitmentID)
		} else {
			n.LogError("failed to load commitment", "commitmentID", commitmentID)
			panic("here4")
		}

		return
	}

	if commitment.ID() != commitmentID {
		n.LogTrace("requested commitment does not belong to main engine", "requestedCommitmentID", commitmentID, "mainEngineCommitmentID", commitment.ID())
		return
	}

	attestations, err := mainEngine.Attestations.Get(commitmentID.Index())
	if err != nil {
		n.LogError("failed to load attestations", "commitmentID", commitmentID, "error", err)
		panic("here")
		return
	}

	rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Index())
	if err != nil {
		n.LogError("failed to load roots", "commitmentID", commitmentID, "error", err)
		panic("here1")
		return
	}

	roots, err := rootsStorage.Load(commitmentID)
	if err != nil {
		n.LogError("failed to load roots", "commitmentID", commitmentID, "error", err)
		panic("here2")
		return
	}

	n.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
}

func (n *Network) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], peer peer.ID) {
	n.processTask("warp sync response", func() (err error, logLevel log.Level) {
		chainCommitment, err := n.protocol.Commitment(commitmentID)
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

		n.warpSyncRequester.StopTicker(commitmentID)

		for _, blockID := range blockIDs {
			targetEngine.BlockDAG.GetOrRequestBlock(blockID)
		}

		return nil, log.LevelDebug
	}, "commitmentID", commitmentID, "blockIDs", blockIDs, "proof", proof, "peer", peer)
}

func (n *Network) processTask(taskName string, task func() (err error, logLevel log.Level), args ...any) {
	if err, logLevel := task(); err != nil {
		n.Log("failed to process "+taskName, logLevel, append(args, "error", err)...)
	} else {
		n.Log("successfully processed "+taskName, logLevel, args...)
	}
}

func (n *Network) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.processTask("warp sync request", func() (err error, logLevel log.Level) {
		committedSlot, err := n.protocol.MainEngineInstance().CommittedSlot(commitmentID)
		if err != nil {
			return ierrors.Wrap(err, "failed to get slot for commitment"), log.LevelDebug
		}

		commitment, err := committedSlot.Commitment()
		if err != nil {
			return ierrors.Wrap(err, "failed to get commitment from slot"), log.LevelDebug
		} else if commitment.ID() != commitmentID {
			return ierrors.Errorf("commitment ID mismatch: %s != %s", commitment.ID(), commitmentID), log.LevelDebug
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			return ierrors.Wrap(err, "failed to get block IDs from slot"), log.LevelDebug
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			return ierrors.Wrap(err, "failed to get roots from slot"), log.LevelDebug
		}

		n.protocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), src)

		return
	}, "commitmentID", commitmentID, "peer", src)
}

func (n *Network) OnSendBlock(callback func(block *model.Block)) (unsubscribe func()) {
	return n.gossip.Hook(callback).Unhook
}

func (n *Network) OnBlockRequested(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return n.blockRequested.Hook(callback).Unhook
}

func (n *Network) OnBlockRequestStarted(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return n.blockRequestStarted.Hook(callback).Unhook
}

func (n *Network) OnBlockRequestStopped(callback func(blockID iotago.BlockID, engine *engine.Engine)) (unsubscribe func()) {
	return n.blockRequestStopped.Hook(callback).Unhook
}

func (n *Network) OnCommitmentRequestStarted(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.commitmentRequester.Events.TickerStarted.Hook(callback).Unhook
}

func (n *Network) OnCommitmentRequestStopped(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.commitmentRequester.Events.TickerStopped.Hook(callback).Unhook
}

func (n *Network) OnCommitmentRequested(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.commitmentRequester.Events.Tick.Hook(callback).Unhook
}

func (n *Network) OnAttestationsRequestStarted(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.attestationsRequester.Events.TickerStarted.Hook(callback).Unhook
}

func (n *Network) OnAttestationsRequestStopped(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.attestationsRequester.Events.TickerStopped.Hook(callback).Unhook
}

func (n *Network) OnAttestationsRequested(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.attestationsRequester.Events.Tick.Hook(callback).Unhook
}

func (n *Network) Shutdown() {}

func (n *Network) startAttestationsRequester() {
	n.protocol.HookConstructed(func() {
		n.protocol.ChainCreated.Hook(func(chain *Chain) {
			chain.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					n.commitmentVerifiers.GetOrCreate(chain.ForkingPoint.Get().ID(), func() *CommitmentVerifier {
						return NewCommitmentVerifier(chain.Engine.Get(), chain.ForkingPoint.Get().Parent.Get().Commitment)
					})
				} else {
					n.commitmentVerifiers.Delete(chain.ForkingPoint.Get().ID())
				}
			})
		})

		n.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				if requestAttestations {
					if commitment.CumulativeWeight() == 0 {
						commitment.IsAttested.Set(true)
					} else {
						n.attestationsRequester.StartTicker(commitment.ID())
					}
				} else {
					n.attestationsRequester.StopTicker(commitment.ID())
				}
			})
		})
	})
}

func (n *Network) startWarpSyncRequester() {
	n.protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.WarpSyncBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
			if warpSyncBlocks {
				n.warpSyncRequester.StartTicker(commitment.ID())
			} else {
				n.warpSyncRequester.StopTicker(commitment.ID())
			}
		})
	})
}

func (n *Network) startBlockRequester() {
	startBlockRequester := func(engine *engine.Engine) {
		unsubscribe := lo.Batch(
			engine.Events.BlockRequester.Tick.Hook(func(id iotago.BlockID) {
				n.blockRequested.Trigger(id, engine)
			}).Unhook,

			engine.Events.BlockRequester.TickerStarted.Hook(func(id iotago.BlockID) {
				n.blockRequestStarted.Trigger(id, engine)
			}).Unhook,

			engine.Events.BlockRequester.TickerStopped.Hook(func(id iotago.BlockID) {
				n.blockRequestStopped.Trigger(id, engine)
			}).Unhook,
		)

		engine.HookShutdown(unsubscribe)
	}

	n.protocol.MainChain.Get().Engine.OnUpdate(func(_, engine *engine.Engine) {
		startBlockRequester(engine)
	})

	n.protocol.ChainCreated.Hook(func(chain *Chain) {
		chain.Engine.OnUpdate(func(_, engine *engine.Engine) { startBlockRequester(engine) })
	})
}
