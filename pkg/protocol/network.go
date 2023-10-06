package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/buffer"
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
	droppedBlocksBuffer   *buffer.UnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]]

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
		droppedBlocksBuffer:   buffer.NewUnsolidCommitmentBuffer[*types.Tuple[*model.Block, peer.ID]](20, 100),
	}

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

	protocol.HookConstructed(func() {
		protocol.CommitmentCreated.Hook(func(commitment *Commitment) {
			commitment.InSyncRange.OnUpdate(func(_, inSyncRange bool) {
				if inSyncRange {
					for _, droppedBlock := range n.droppedBlocksBuffer.GetValues(commitment.ID()) {
						// TODO: replace with workerpool
						go n.ProcessReceivedBlock(droppedBlock.A, droppedBlock.B)
					}
				}
			})
		})
	})

	protocol.HookInitialized(func() {
		n.OnError(func(err error, peer peer.ID) {
			n.protocol.LogError("network error", "peer", peer, "error", err)
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

			n.warpSyncRequester.Events.Tick.Hook(n.SendWarpSyncRequest).Unhook,
			n.OnSendBlock(func(block *model.Block) { n.SendBlock(block) }),
			n.OnBlockRequested(func(blockID iotago.BlockID, engine *engine.Engine) {
				n.protocol.LogDebug("block requested", "blockID", blockID, "engine", engine.Name())

				n.RequestBlock(blockID)
			}),
			n.OnCommitmentRequested(func(id iotago.CommitmentID) {
				n.protocol.LogDebug("commitment requested", "commitmentID", id)

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

func (n *Network) SendWarpSyncRequest(id iotago.CommitmentID) {
	n.protocol.LogDebug("request warp sync", "commitmentID", id)

	n.Protocol.SendWarpSyncRequest(id)
}

func (n *Network) IssueBlock(block *model.Block) error {
	n.protocol.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (n *Network) ProcessReceivedBlock(block *model.Block, src peer.ID) {
	n.processTask("received block", func() (logLevel log.Level, err error) {
		logLevel = log.LevelTrace

		commitmentRequest := n.protocol.requestCommitment(block.ProtocolBlock().SlotCommitmentID, true)
		if !commitmentRequest.WasCompleted() {
			if !n.droppedBlocksBuffer.Add(block.ProtocolBlock().SlotCommitmentID, types.NewTuple(block, src)) {
				return log.LevelError, ierrors.New("failed to add block to dropped blocks buffer")
			}

			return logLevel, ierrors.Errorf("referenced commitment %s unknown", block.ProtocolBlock().SlotCommitmentID)
		}

		if commitmentRequest.WasRejected() {
			return logLevel, commitmentRequest.Err()
		}

		if chain := commitmentRequest.Result().Chain.Get(); chain != nil {
			if chain.DispatchBlock(block, src) {
				n.protocol.LogError("block dropped", "blockID", block.ID(), "peer", src)
			}
		}

		return logLevel, nil
	}, "blockID", block.ID(), "peer", src)
}

func (n *Network) ProcessBlockRequest(blockID iotago.BlockID, src peer.ID) {
	n.processTask("block request", func() (logLevel log.Level, err error) {
		block, exists := n.protocol.MainEngineInstance().Block(blockID)
		if !exists {
			return log.LevelTrace, ierrors.Errorf("requested block %s not found", blockID)
		}

		n.protocol.SendBlock(block, src)

		return log.LevelTrace, nil
	}, "blockID", blockID, "peer", src)
}

func (n *Network) ProcessCommitment(commitmentModel *model.Commitment, peer peer.ID) {
	n.processTask("commitment", func() (logLevel log.Level, err error) {
		_, published, err := n.protocol.PublishCommitment(commitmentModel)
		if err != nil {
			return log.LevelError, ierrors.Wrapf(err, "failed to publish commitment")
		}

		if !published {
			return log.LevelTrace, ierrors.New("commitment published previously")
		}

		return log.LevelDebug, nil
	}, "commitmentID", commitmentModel.ID(), "peer", peer)
}

func (n *Network) ProcessCommitmentRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.protocol.LogTrace("commitment request received", "commitmentID", commitmentID, "peer", src)

	if commitment, err := n.protocol.Commitment(commitmentID); err != nil {
		if !ierrors.Is(err, ErrorCommitmentNotFound) {
			n.protocol.LogDebug("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		} else {
			n.protocol.LogTrace("failed to process commitment request", "commitmentID", commitmentID, "peer", src, "error", err)
		}
	} else {
		n.protocol.LogTrace("sending commitment", "commitmentID", commitmentID, "peer", src)

		n.protocol.SendSlotCommitment(commitment.Commitment, src)
	}
}

func (n *Network) ProcessAttestations(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) {
	commitment, _, err := n.protocol.PublishCommitment(commitmentModel)
	if err != nil {
		n.protocol.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitmentModel.ID(), "peer", source, "error", err)
		return
	}

	if !commitment.RequestAttestations.Get() {
		n.protocol.LogTrace("received attestations for previously attested commitment", "commitment", commitment.LogName())
		return
	}

	chain := commitment.Chain.Get()
	if chain == nil {
		n.protocol.LogDebug("failed to find chain for commitment when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	commitmentVerifier, exists := n.commitmentVerifiers.Get(chain.ForkingPoint.Get().ID())
	if !exists {
		n.protocol.LogDebug("failed to find commitment verifier for commitment %s when processing attestations", "commitmentID", commitmentModel.ID())
		return
	}

	_, actualWeight, err := commitmentVerifier.verifyCommitment(commitment, attestations, merkleProof)
	if err != nil {
		n.protocol.LogError("failed to verify commitment when processing attestations", "commitmentID", commitmentModel.ID(), "error", err)
		return
	}

	commitment.AttestedWeight.Set(actualWeight)
	commitment.IsAttested.Set(true)
}

func (n *Network) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.processTask("attestations request", func() (logLevel log.Level, err error) {
		mainEngine := n.protocol.MainEngineInstance()

		if mainEngine.Storage.Settings().LatestCommitment().Slot() < commitmentID.Slot() {
			return log.LevelTrace, ierrors.New("requested commitment is not verified, yet")
		}

		commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Slot())
		if err != nil {
			return lo.Cond(ierrors.Is(err, kvstore.ErrKeyNotFound), log.LevelTrace, log.LevelError), ierrors.Wrapf(err, "failed to load commitment")
		}

		if commitment.ID() != commitmentID {
			return log.LevelTrace, ierrors.Errorf("requested commitment %s does not match main engine commitment %s", commitmentID, commitment.ID())
		}

		attestations, err := mainEngine.Attestations.Get(commitmentID.Slot())
		if err != nil {
			return log.LevelError, ierrors.Wrapf(err, "failed to load attestations")
		}

		rootsStorage, err := mainEngine.Storage.Roots(commitmentID.Slot())
		if err != nil {
			return log.LevelError, ierrors.Wrapf(err, "failed to load roots")
		}

		roots, err := rootsStorage.Load(commitmentID)
		if err != nil {
			return log.LevelError, ierrors.Wrapf(err, "failed to load roots")
		}

		return log.LevelDebug, n.protocol.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
	}, "commitmentID", commitmentID, "peer", src)
}

func (n *Network) ProcessWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], peer peer.ID) {
	n.processTask("warp sync response", func() (logLevel log.Level, err error) {
		logLevel = log.LevelTrace

		chainCommitment, err := n.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				logLevel = log.LevelError
			}

			return logLevel, ierrors.Wrapf(err, "failed to get commitment")
		}

		targetEngine := chainCommitment.Engine.Get()
		if targetEngine == nil {
			return log.LevelDebug, ierrors.New("failed to get target engine")
		}

		chainCommitment.RequestedBlocksReceived.Compute(func(requestedBlocksReceived bool) bool {
			if requestedBlocksReceived || !chainCommitment.RequestBlocks.Get() {
				err = ierrors.New("warp sync not requested")
				return requestedBlocksReceived
			}

			acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
			for _, blockID := range blockIDs {
				_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
			}

			if !iotago.VerifyProof(proof, iotago.Identifier(acceptedBlocks.Root()), chainCommitment.RootsID()) {
				logLevel, err = log.LevelError, ierrors.New("failed to verify merkle proof")
				return false
			}

			n.warpSyncRequester.StopTicker(commitmentID)

			for _, blockID := range blockIDs {
				targetEngine.BlockDAG.GetOrRequestBlock(blockID)
			}

			logLevel = log.LevelDebug

			return true
		})

		return logLevel, err
	}, "commitmentID", commitmentID, "blockIDs", blockIDs, "proof", proof, "peer", peer)
}

func (n *Network) processTask(taskName string, task func() (logLevel log.Level, err error), args ...any) {
	if logLevel, err := task(); err != nil {
		n.protocol.Log("failed to process "+taskName, logLevel, append(args, "error", err)...)
	} else {
		n.protocol.Log("successfully processed "+taskName, logLevel, args...)
	}
}

func (n *Network) ProcessWarpSyncRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.processTask("warp sync request", func() (logLevel log.Level, err error) {
		logLevel = log.LevelTrace

		commitment, err := n.protocol.Commitment(commitmentID)
		if err != nil {
			if !ierrors.Is(err, ErrorCommitmentNotFound) {
				logLevel = log.LevelError
			}

			return logLevel, ierrors.Wrap(err, "failed to load commitment")
		}

		chain := commitment.Chain.Get()
		if chain == nil {
			return logLevel, ierrors.New("requested commitment is not solid")
		}

		engine := commitment.Engine.Get()
		if engine == nil {
			return logLevel, ierrors.New("requested commitment does not have an engine, yet")
		}

		committedSlot, err := engine.CommittedSlot(commitmentID)
		if err != nil {
			return logLevel, ierrors.Wrap(err, "failed to get slot for commitment")
		}

		blockIDs, err := committedSlot.BlockIDs()
		if err != nil {
			return log.LevelError, ierrors.Wrap(err, "failed to get block IDs from slot")
		}

		roots, err := committedSlot.Roots()
		if err != nil {
			return logLevel, ierrors.Wrap(err, "failed to get roots from slot")
		}

		n.protocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.TangleProof(), src)

		return logLevel, nil
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
		n.protocol.OnChainCreated(func(chain *Chain) {
			chain.RequestAttestations.OnUpdate(func(_, requestAttestations bool) {
				forkingPoint := chain.ForkingPoint.Get()

				if requestAttestations {
					if commitmentBeforeForkingPoint := forkingPoint.Parent.Get(); commitmentBeforeForkingPoint != nil {
						n.commitmentVerifiers.GetOrCreate(forkingPoint.ID(), func() *CommitmentVerifier {
							return NewCommitmentVerifier(chain.Engine.Get(), commitmentBeforeForkingPoint.Commitment)
						})
					}
				} else {
					n.commitmentVerifiers.Delete(forkingPoint.ID())
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
		commitment.RequestBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
			if warpSyncBlocks {
				n.warpSyncRequester.StartTicker(commitment.ID())
			} else {
				n.warpSyncRequester.StopTicker(commitment.ID())
			}
		})
	})
}

func (n *Network) startBlockRequester() {
	n.protocol.Chains.Chains.OnUpdate(func(mutations ds.SetMutations[*Chain]) {
		mutations.AddedElements().Range(func(chain *Chain) {
			chain.Engine.OnUpdate(func(_, engine *engine.Engine) {
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
			})
		})
	})
}
