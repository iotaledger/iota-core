package protocol

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Network struct {
	protocol *Protocol

	*core.Protocol

	module.Module
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *Network {
	n := &Network{
		protocol: protocol,
		Protocol: core.NewProtocol(endpoint, protocol.Workers().CreatePool("NetworkProtocol"), protocol),
	}

	protocol.HookConstructed(func() {
		n.OnError(func(err error, src network.PeerID) {
			protocol.TriggerError(ierrors.Wrapf(err, "Network error from %s", src))
		})
	})

	protocol.HookInitialized(func() {
		n.startNetwork()

		n.TriggerInitialized()
	})

	protocol.HookShutdown(func() {
		n.TriggerShutdown()

		n.Shutdown()

		n.TriggerStopped()
	})

	n.TriggerConstructed()

	return n
}

func (n *Network) startNetwork() {
	n.OnBlockReceived(func(block *model.Block, src network.PeerID) {
		panic("implement me")
	})

	n.OnBlockRequestReceived(func(blockID iotago.BlockID, src network.PeerID) {
		if block, exists := n.protocol.MainEngine().Block(blockID); !exists {
			n.SendBlock(block, src)
		}
	})

	n.OnSlotCommitmentReceived(func(commitment *model.Commitment, src network.PeerID) {
		panic("implement me")
	})

	n.OnSlotCommitmentRequestReceived(func(commitmentID iotago.CommitmentID, src network.PeerID) {
		n.protocol.ProcessSlotCommitmentRequest(commitmentID, src)
	})

	n.OnAttestationsReceived(func(commitment *model.Commitment, attestations []*iotago.Attestation, m *merklehasher.Proof[iotago.Identifier], id network.PeerID) {
		n.protocol.ProcessAttestationsResponse(commitment, attestations, m, id)
	})

	n.OnAttestationsRequestReceived(func(commitmentID iotago.CommitmentID, src network.PeerID) {
		n.protocol.ProcessAttestationsRequest(commitmentID, src)
	})

	n.OnWarpSyncResponseReceived(func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src network.PeerID) {
		panic("implement me")
	})

	n.OnWarpSyncRequestReceived(func(commitmentID iotago.CommitmentID, src network.PeerID) {
		panic("implement me")
	})

	// TODO: WIRE UP NETWORK EVENTS
}

func (n *Network) OnBlockReceived(callback func(block *model.Block, src network.PeerID)) (unsubscribe func()) {
	return n.Events.BlockReceived.Hook(callback).Unhook
}

func (n *Network) OnBlockRequestReceived(callback func(blockID iotago.BlockID, src network.PeerID)) (unsubscribe func()) {
	return n.Events.BlockRequestReceived.Hook(callback).Unhook
}

func (n *Network) OnSlotCommitmentReceived(callback func(commitment *model.Commitment, src network.PeerID)) (unsubscribe func()) {
	return n.Events.SlotCommitmentReceived.Hook(callback).Unhook
}

func (n *Network) OnSlotCommitmentRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.Events.SlotCommitmentRequestReceived.Hook(callback).Unhook
}

func (n *Network) OnAttestationsReceived(callback func(*model.Commitment, []*iotago.Attestation, *merklehasher.Proof[iotago.Identifier], network.PeerID)) (unsubscribe func()) {
	return n.Events.AttestationsReceived.Hook(callback).Unhook
}

func (n *Network) OnAttestationsRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.Events.AttestationsRequestReceived.Hook(callback).Unhook
}

func (n *Network) OnWarpSyncResponseReceived(callback func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src network.PeerID)) (unsubscribe func()) {
	return n.Events.WarpSyncResponseReceived.Hook(callback).Unhook
}

func (n *Network) OnWarpSyncRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.Events.WarpSyncRequestReceived.Hook(callback).Unhook
}

func (n *Network) OnError(callback func(err error, src network.PeerID)) (unsubscribe func()) {
	return n.Events.Error.Hook(callback).Unhook
}
