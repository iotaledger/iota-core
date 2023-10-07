package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
)

type NetworkManager struct {
	protocol *Protocol

	Network               *core.Protocol
	AttestationsRequester *AttestationsRequester
	BlockRequester        *BlockRequester
	CommitmentRequester   *CommitmentRequester

	shutdown reactive.Event
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *NetworkManager {
	n := &NetworkManager{
		protocol:              protocol,
		Network:               core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
		BlockRequester:        NewBlockRequester(protocol),
		CommitmentRequester:   NewCommitmentRequester(protocol),
		AttestationsRequester: NewAttestationsRequester(protocol),

		shutdown: reactive.NewEvent(),
	}

	protocol.HookInitialized(func() {
		unsubscribeFromNetworkEvents := lo.Batch(
			n.Network.OnError(func(err error, peer peer.ID) {
				n.protocol.LogError("network error", "peer", peer, "error", err)
			}),

			// inbound: Network -> GossipProtocol
			n.Network.OnWarpSyncResponseReceived(protocol.GossipProtocol.ProcessWarpSyncResponse),
			n.Network.OnWarpSyncRequestReceived(protocol.GossipProtocol.ProcessWarpSyncRequest),

			// outbound: GossipProtocol -> Network

			n.protocol.warpSyncRequester.Events.Tick.Hook(protocol.GossipProtocol.SendWarpSyncRequest).Unhook,

			n.Network.OnCommitmentReceived(protocol.CommitmentRequester.ProcessResponse),
			n.Network.OnCommitmentRequestReceived(protocol.CommitmentRequester.ProcessRequest),
			n.Network.OnBlockReceived(protocol.BlockRequester.ProcessResponse),
			n.Network.OnBlockRequestReceived(protocol.BlockRequester.ProcessRequest),
			n.Network.OnAttestationsReceived(protocol.AttestationsRequester.ProcessResponse),
			n.Network.OnAttestationsRequestReceived(protocol.AttestationsRequester.ProcessRequest),
		)

		protocol.HookShutdown(func() {
			unsubscribeFromNetworkEvents()

			protocol.inboundWorkers.Shutdown().ShutdownComplete.Wait()
			protocol.outboundWorkers.Shutdown().ShutdownComplete.Wait()

			n.BlockRequester.Shutdown()
			n.AttestationsRequester.Shutdown()

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
