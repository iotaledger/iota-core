package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
)

type NetworkManager struct {
	protocol *Protocol

	Network      *core.Protocol
	Attestations *AttestationsProtocol
	Blocks       *BlocksProtocol
	Commitments  *CommitmentsProtocol
	WarpSync     *WarpSyncProtocol
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *NetworkManager {
	n := &NetworkManager{
		protocol:     protocol,
		Network:      core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
		Blocks:       NewBlockRequester(protocol),
		Commitments:  NewCommitmentRequester(protocol),
		Attestations: NewAttestationsRequester(protocol),
		WarpSync:     NewWarpSyncRequester(protocol),
	}

	protocol.HookInitialized(func() {
		unsubscribeFromNetworkEvents := lo.Batch(
			n.Network.OnError(func(err error, peer peer.ID) { n.protocol.LogError("network error", "peer", peer, "error", err) }),
			n.Network.OnBlockReceived(protocol.Blocks.ProcessResponse),
			n.Network.OnBlockRequestReceived(protocol.Blocks.ProcessRequest),
			n.Network.OnCommitmentReceived(protocol.Commitments.ProcessResponse),
			n.Network.OnCommitmentRequestReceived(protocol.Commitments.ProcessRequest),
			n.Network.OnAttestationsReceived(protocol.Attestations.ProcessResponse),
			n.Network.OnAttestationsRequestReceived(protocol.Attestations.ProcessRequest),
			n.Network.OnWarpSyncResponseReceived(protocol.WarpSync.ProcessResponse),
			n.Network.OnWarpSyncRequestReceived(protocol.WarpSync.ProcessRequest),
		)

		protocol.HookShutdown(func() {
			unsubscribeFromNetworkEvents()

			n.Blocks.Shutdown()
			n.Commitments.Shutdown()
			n.Attestations.Shutdown()
			n.WarpSync.Shutdown()
			n.Network.Shutdown()
		})
	})

	return n
}

func (n *NetworkManager) IssueBlock(block *model.Block) error {
	n.Blocks.ProcessResponse(block, "self")

	return nil
}

func (n *NetworkManager) Shutdown() {}
