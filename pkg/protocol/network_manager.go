package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/core/eventticker"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type NetworkManager struct {
	*Protocol

	Network *core.Protocol

	attestationsRequester *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	commitmentRequester   *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	warpSyncRequester     *eventticker.EventTicker[iotago.SlotIndex, iotago.CommitmentID]
	blockRequestStarted   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequestStopped   *event.Event2[iotago.BlockID, *engine.Engine]
	blockRequested        *event.Event2[iotago.BlockID, *engine.Engine]
	commitmentVerifiers   *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *CommitmentVerifier]
	shutdown              reactive.Event
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *NetworkManager {
	n := &NetworkManager{
		Protocol:              protocol,
		Network:               core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
		attestationsRequester: eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		commitmentRequester:   eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		warpSyncRequester:     eventticker.New[iotago.SlotIndex, iotago.CommitmentID](),
		blockRequestStarted:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequestStopped:   event.New2[iotago.BlockID, *engine.Engine](),
		blockRequested:        event.New2[iotago.BlockID, *engine.Engine](),
		commitmentVerifiers:   shrinkingmap.New[iotago.CommitmentID, *CommitmentVerifier](),
		shutdown:              reactive.NewEvent(),
	}

	n.startBlockRequester()
	n.startAttestationsRequester()
	n.startWarpSyncRequester()

	for _, gossipEvent := range []*event.Event1[*blocks.Block]{
		// TODO: REPLACE WITH REACTIVE VERSION
		protocol.Events.Engine.Scheduler.BlockScheduled,
		protocol.Events.Engine.Scheduler.BlockSkipped,
	} {
		gossipEvent.Hook(func(block *blocks.Block) { n.SendBlock(block.ModelBlock()) })
	}

	var unsubscribeFromNetworkEvents func()

	protocol.HookInitialized(func() {
		n.Network.OnError(func(err error, peer peer.ID) {
			n.LogError("network error", "peer", peer, "error", err)
		})

		unsubscribeFromNetworkEvents = lo.Batch(
			// inbound: Network -> GossipProtocol
			n.Network.OnBlockReceived(n.ProcessBlock),
			n.Network.OnBlockRequestReceived(n.ProcessBlockRequest),
			n.Network.OnCommitmentReceived(n.ProcessCommitment),
			n.Network.OnCommitmentRequestReceived(n.ProcessCommitmentRequest),
			n.Network.OnWarpSyncResponseReceived(n.ProcessWarpSyncResponse),
			n.Network.OnWarpSyncRequestReceived(n.ProcessWarpSyncRequest),

			// outbound: GossipProtocol -> Network
			n.blockRequested.Hook(n.SendBlockRequest).Unhook,
			n.commitmentRequester.Events.Tick.Hook(n.SendCommitmentRequest).Unhook,
			n.attestationsRequester.Events.Tick.Hook(n.SendAttestationsRequest).Unhook,
			n.warpSyncRequester.Events.Tick.Hook(n.SendWarpSyncRequest).Unhook,

			n.Network.OnAttestationsReceived(n.ProcessAttestations),
			n.Network.OnAttestationsRequestReceived(n.ProcessAttestationsRequest),
		)

		protocol.HookShutdown(func() {
			unsubscribeFromNetworkEvents()

			protocol.GossipProtocol.inboundWorkers.Shutdown().ShutdownComplete.Wait()
			protocol.GossipProtocol.outboundWorkers.Shutdown().ShutdownComplete.Wait()
			// shutdown gossip and wait

			n.Network.Shutdown()

			n.shutdown.Trigger()
		})
	})

	return n
}

func (n *NetworkManager) OnShutdown(callback func()) (unsubscribe func()) {
	return n.shutdown.OnTrigger(callback)
}

func (n *NetworkManager) IssueBlock(block *model.Block) error {
	n.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (n *NetworkManager) ProcessAttestations(commitmentModel *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], source peer.ID) {
	commitment, _, err := n.PublishCommitment(commitmentModel)
	if err != nil {
		n.LogDebug("failed to publish commitment when processing attestations", "commitmentID", commitmentModel.ID(), "peer", source, "error", err)
		return
	}

	if !commitment.RequestAttestations.Get() {
		n.LogTrace("received attestations for previously attested commitment", "commitment", commitment.LogName())
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

func (n *NetworkManager) ProcessAttestationsRequest(commitmentID iotago.CommitmentID, src peer.ID) {
	n.processTask("attestations request", func() (logLevel log.Level, err error) {
		mainEngine := n.MainEngineInstance()

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

		return log.LevelDebug, n.Network.SendAttestations(commitment, attestations, roots.AttestationsProof(), src)
	}, "commitmentID", commitmentID, "peer", src)
}

func (n *NetworkManager) processTask(taskName string, task func() (logLevel log.Level, err error), args ...any) {
	if logLevel, err := task(); err != nil {
		n.Log("failed to process "+taskName, logLevel, append(args, "error", err)...)
	} else {
		n.Log("successfully processed "+taskName, logLevel, args...)
	}
}

func (n *NetworkManager) OnAttestationsRequestStarted(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.attestationsRequester.Events.TickerStarted.Hook(callback).Unhook
}

func (n *NetworkManager) OnAttestationsRequestStopped(callback func(commitmentID iotago.CommitmentID)) (unsubscribe func()) {
	return n.attestationsRequester.Events.TickerStopped.Hook(callback).Unhook
}

func (n *NetworkManager) Shutdown() {}

func (n *NetworkManager) startAttestationsRequester() {
	n.HookConstructed(func() {
		n.OnChainCreated(func(chain *Chain) {
			chain.CheckAttestations.OnUpdate(func(_, requestAttestations bool) {
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

		n.CommitmentCreated.Hook(func(commitment *Commitment) {
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

func (n *NetworkManager) startWarpSyncRequester() {
	n.CommitmentCreated.Hook(func(commitment *Commitment) {
		commitment.RequestBlocks.OnUpdate(func(_, warpSyncBlocks bool) {
			if warpSyncBlocks {
				n.warpSyncRequester.StartTicker(commitment.ID())
			} else {
				n.warpSyncRequester.StopTicker(commitment.ID())
			}
		})
	})
}

func (n *NetworkManager) startBlockRequester() {
	n.ChainManager.Chains.OnUpdate(func(mutations ds.SetMutations[*Chain]) {
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
