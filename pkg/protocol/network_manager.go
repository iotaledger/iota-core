package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

type NetworkManager struct {
	protocol *Protocol

	Network *core.Protocol

	shutdown reactive.Event
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *NetworkManager {
	n := &NetworkManager{
		protocol: protocol,
		Network:  core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),

		shutdown: reactive.NewEvent(),
	}

	protocol.HookInitialized(func() {
		blockRequested := event.New1[*model.Block]()
		for _, gossipEvent := range []*event.Event1[*blocks.Block]{
			// TODO: REPLACE WITH REACTIVE VERSION
			protocol.Events.Engine.Scheduler.BlockScheduled,
			protocol.Events.Engine.Scheduler.BlockSkipped,
		} {
			gossipEvent.Hook(func(block *blocks.Block) { blockRequested.Trigger(block.ModelBlock()) })
		}

		unsubscribeFromNetworkEvents := lo.Batch(
			n.Network.OnError(func(err error, peer peer.ID) {
				n.protocol.LogError("network error", "peer", peer, "error", err)
			}),

			// inbound: Network -> GossipProtocol
			n.Network.OnBlockReceived(protocol.GossipProtocol.ProcessBlock),
			n.Network.OnBlockRequestReceived(protocol.GossipProtocol.ProcessBlockRequest),
			n.Network.OnCommitmentReceived(protocol.GossipProtocol.ProcessCommitment),
			n.Network.OnCommitmentRequestReceived(protocol.GossipProtocol.ProcessCommitmentRequest),
			n.Network.OnWarpSyncResponseReceived(protocol.GossipProtocol.ProcessWarpSyncResponse),
			n.Network.OnWarpSyncRequestReceived(protocol.GossipProtocol.ProcessWarpSyncRequest),

			// outbound: GossipProtocol -> Network
			blockRequested.Hook(protocol.GossipProtocol.SendBlock).Unhook,
			n.protocol.blockRequested.Hook(protocol.GossipProtocol.SendBlockRequest).Unhook,
			n.protocol.commitmentRequester.Events.Tick.Hook(protocol.GossipProtocol.SendCommitmentRequest).Unhook,

			n.protocol.warpSyncRequester.Events.Tick.Hook(protocol.GossipProtocol.SendWarpSyncRequest).Unhook,

			n.Network.OnAttestationsReceived(protocol.AttestationsRequester.ProcessResponse),
			n.Network.OnAttestationsRequestReceived(protocol.AttestationsRequester.ProcessRequest),
		)

		protocol.HookShutdown(func() {
			unsubscribeFromNetworkEvents()

			protocol.inboundWorkers.Shutdown().ShutdownComplete.Wait()
			protocol.outboundWorkers.Shutdown().ShutdownComplete.Wait()

			protocol.AttestationsRequester.Shutdown()

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
	n.protocol.MainEngineInstance().ProcessBlockFromPeer(block, "self")

	return nil
}

func (n *NetworkManager) Shutdown() {}
