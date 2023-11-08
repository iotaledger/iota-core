package core

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	nwmodels "github.com/iotaledger/iota-core/pkg/network/protocols/core/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

type WarpSyncPayload struct {
	BlockIDsBySlotCommitmentID map[iotago.CommitmentID]iotago.BlockIDs `serix:",lenPrefix=uint32"`
	TangleMerkleProof          *merklehasher.Proof[iotago.Identifier]  `serix:""`
	TransactionIDs             iotago.TransactionIDs                   `serix:""`
	MutationsMerkleProof       *merklehasher.Proof[iotago.Identifier]  `serix:""`
}

func (p *Protocol) SendWarpSyncRequest(id iotago.CommitmentID, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_WarpSyncRequest{
		WarpSyncRequest: &nwmodels.WarpSyncRequest{
			CommitmentId: lo.PanicOnErr(id.Bytes()),
		},
	}}, to...)
}

func (p *Protocol) SendWarpSyncResponse(id iotago.CommitmentID, blockIDsBySlotCommitmentID map[iotago.CommitmentID]iotago.BlockIDs, tangleMerkleProof *merklehasher.Proof[iotago.Identifier], transactionIDs iotago.TransactionIDs, mutationsMerkleProof *merklehasher.Proof[iotago.Identifier], to ...peer.ID) {
	serializer := p.apiProvider.APIForSlot(id.Slot())

	payload := &WarpSyncPayload{
		BlockIDsBySlotCommitmentID: blockIDsBySlotCommitmentID,
		TangleMerkleProof:          tangleMerkleProof,
		TransactionIDs:             transactionIDs,
		MutationsMerkleProof:       mutationsMerkleProof,
	}

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_WarpSyncResponse{
		WarpSyncResponse: &nwmodels.WarpSyncResponse{
			CommitmentId: lo.PanicOnErr(id.Bytes()),
			Payload:      lo.PanicOnErr(serializer.Encode(payload)),
		},
	}}, to...)
}

func (p *Protocol) handleWarpSyncRequest(commitmentIDBytes []byte, id peer.ID) {
	p.workerPool.Submit(func() {
		commitmentID, _, err := iotago.CommitmentIDFromBytes(commitmentIDBytes)
		if err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitmentID in warp sync request"), id)

			return
		}

		p.Events.WarpSyncRequestReceived.Trigger(commitmentID, id)
	})
}

func (p *Protocol) handleWarpSyncResponse(commitmentIDBytes []byte, payloadBytes []byte, id peer.ID) {
	p.workerPool.Submit(func() {
		commitmentID, _, err := iotago.CommitmentIDFromBytes(commitmentIDBytes)
		if err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitmentID in warp sync response"), id)

			return
		}

		payload := new(WarpSyncPayload)
		if _, err = p.apiProvider.APIForSlot(commitmentID.Slot()).Decode(payloadBytes, payload, serix.WithValidation()); err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize payload"), id)

			return
		}

		p.Events.WarpSyncResponseReceived.Trigger(commitmentID, payload.BlockIDsBySlotCommitmentID, payload.TangleMerkleProof, payload.TransactionIDs, payload.MutationsMerkleProof, id)
	})
}
