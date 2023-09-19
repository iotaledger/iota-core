package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Network struct {
	protocol *Protocol

	*core.Protocol

	module.Module
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *Network {
	n := &Network{
		protocol: protocol,
		Protocol: core.NewProtocol(endpoint, protocol.Workers.CreatePool("NetworkProtocol"), protocol),
	}

	var unsubscribeFromNetworkEvents func()

	protocol.HookInitialized(func() {
		n.OnError(func(err error, src peer.ID) {
			protocol.LogError("network error", "peer", src, "error", err)
		})

		unsubscribeFromNetworkEvents = lo.Batch(
			n.OnBlockReceived(protocol.ProcessBlock),
			n.OnBlockRequestReceived(protocol.ProcessBlockRequest),
			n.OnCommitmentReceived(protocol.ProcessCommitment),
			n.OnCommitmentRequestReceived(protocol.ProcessCommitmentRequest),
			n.OnAttestationsReceived(protocol.ProcessAttestations),
			n.OnAttestationsRequestReceived(protocol.ProcessAttestationsRequest),
			n.OnWarpSyncResponseReceived(protocol.ProcessWarpSyncResponse),
			n.OnWarpSyncRequestReceived(protocol.ProcessWarpSyncRequest),

			protocol.OnSendBlock(func(block *model.Block) { n.SendBlock(block) }),
			protocol.OnBlockRequested(func(blockID iotago.BlockID, engine *engine.Engine) { n.RequestBlock(blockID) }),
			protocol.OnCommitmentRequested(func(id iotago.CommitmentID) { n.RequestSlotCommitment(id) }),
			protocol.OnAttestationsRequested(func(commitmentID iotago.CommitmentID) { n.RequestAttestations(commitmentID) }),
		)

		n.TriggerInitialized()
	})

	protocol.HookShutdown(func() {
		n.TriggerShutdown()

		unsubscribeFromNetworkEvents()

		n.Protocol.Shutdown()

		n.TriggerStopped()
	})

	n.TriggerConstructed()

	return n
}

func (n *Network) Shutdown() {}
