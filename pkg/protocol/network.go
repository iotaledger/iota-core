package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Network struct {
	*core.Protocol

	module.Module
}

func newNetwork(protocol *Protocol, endpoint network.Endpoint) *Network {
	n := &Network{
		Protocol: core.NewProtocol(endpoint, protocol.Workers().CreatePool("NetworkProtocol"), protocol),
	}

	protocol.HookInitialized(func() {
		n.OnError(func(err error, src peer.ID) {
			protocol.TriggerError(ierrors.Wrapf(err, "Network error from %s", src))
		})

		n.dispatchInboundMessages(protocol)
		n.dispatchOutboundMessages(protocol)

		n.TriggerInitialized()
	})

	protocol.HookShutdown(n.Shutdown)

	n.TriggerConstructed()

	return n
}

func (n *Network) dispatchInboundMessages(protocol *Protocol) {
	n.OnBlockReceived(func(block *model.Block, src peer.ID) {
		if err := protocol.ProcessBlock(block, src); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process block %s from peer %s", block.ID(), src))
		}
	})

	n.OnBlockRequestReceived(func(blockID iotago.BlockID, src peer.ID) {
		if err := protocol.ProcessBlockRequest(blockID, src); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process block %s from peer %s", blockID, src))
		}
	})

	n.OnSlotCommitmentReceived(func(commitment *model.Commitment, src peer.ID) {
		if _, err := protocol.PublishCommitment(commitment); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process slot commitment %s from peer %s", commitment.ID(), src))
		}
	})

	n.OnSlotCommitmentRequestReceived(func(commitmentID iotago.CommitmentID, src peer.ID) {
		if err := protocol.ProcessCommitmentRequest(commitmentID, src); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process slot commitment request %s from peer %s", commitmentID, src))
		}
	})

	n.OnAttestationsReceived(func(commitment *model.Commitment, attestations []*iotago.Attestation, m *merklehasher.Proof[iotago.Identifier], id peer.ID) {
		if err := protocol.ProcessAttestationsResponse(commitment, attestations, m, id); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process attestations response for commitment %s from peer %s", commitment.ID(), id))
		}
	})

	n.OnAttestationsRequestReceived(func(commitmentID iotago.CommitmentID, src peer.ID) {
		if err := protocol.ProcessAttestationsRequest(commitmentID, src); err != nil {
			protocol.TriggerError(ierrors.Wrapf(err, "failed to process attestations request for commitment %s from peer %s", commitmentID, src))
		}
	})

	n.OnWarpSyncResponseReceived(func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src peer.ID) {
		panic("implement me")
	})

	n.OnWarpSyncRequestReceived(func(commitmentID iotago.CommitmentID, src peer.ID) {
		panic("implement me")
	})
}

func (n *Network) dispatchOutboundMessages(protocol *Protocol) {
	protocol.OnSendBlock(func(block *model.Block) {
		n.SendBlock(block)
	})

	protocol.OnBlockRequested(func(blockID iotago.BlockID, engine *engine.Engine) {
		n.RequestBlock(blockID)
	})

	protocol.OnCommitmentRequested(func(id iotago.CommitmentID) {
		n.RequestSlotCommitment(id)
	})

	protocol.OnAttestationsRequested(func(commitmentID iotago.CommitmentID) {
		n.RequestAttestations(commitmentID)
	})

}

func (n *Network) Shutdown() {
	n.TriggerShutdown()
	n.Protocol.Shutdown()
	n.TriggerStopped()
}
