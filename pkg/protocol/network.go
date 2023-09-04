package protocol

import (
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/network/protocols/core"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type Network struct {
	events *core.Events
}

func (n *Network) OnBlockReceived(callback func(block *model.Block, src network.PeerID)) (unsubscribe func()) {
	return n.events.BlockReceived.Hook(callback).Unhook
}

func (n *Network) OnBlockRequestReceived(callback func(blockID iotago.BlockID, src network.PeerID)) (unsubscribe func()) {
	return n.events.BlockRequestReceived.Hook(callback).Unhook
}

func (n *Network) OnSlotCommitmentReceived(callback func(commitment *model.Commitment, src network.PeerID)) (unsubscribe func()) {
	return n.events.SlotCommitmentReceived.Hook(callback).Unhook
}

func (n *Network) OnSlotCommitmentRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.events.SlotCommitmentRequestReceived.Hook(callback).Unhook
}

func (n *Network) AttestationsReceived(callback func(*model.Commitment, []*iotago.Attestation, *merklehasher.Proof[iotago.Identifier], network.PeerID)) (unsubscribe func()) {
	return n.events.AttestationsReceived.Hook(callback).Unhook
}

func (n *Network) AttestationsRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.events.AttestationsRequestReceived.Hook(callback).Unhook
}

func (n *Network) WarpSyncRequestReceived(callback func(commitmentID iotago.CommitmentID, src network.PeerID)) (unsubscribe func()) {
	return n.events.WarpSyncRequestReceived.Hook(callback).Unhook
}

func (n *Network) WarpSyncResponseReceived(callback func(commitmentID iotago.CommitmentID, blockIDs iotago.BlockIDs, proof *merklehasher.Proof[iotago.Identifier], src network.PeerID)) (unsubscribe func()) {
	return n.events.WarpSyncResponseReceived.Hook(callback).Unhook
}

func (n *Network) OnError(callback func(err error, src network.PeerID)) (unsubscribe func()) {
	return n.events.Error.Hook(callback).Unhook
}
