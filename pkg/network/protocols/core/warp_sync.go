package core

import (
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	nwmodels "github.com/iotaledger/iota-core/pkg/network/protocols/core/models"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

func (p *Protocol) SendWarpSyncRequest(id iotago.CommitmentID, to ...peer.ID) {
	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_WarpSyncRequest{
		WarpSyncRequest: &nwmodels.WarpSyncRequest{
			CommitmentId: lo.PanicOnErr(id.Bytes()),
		},
	}}, to...)
}

func (p *Protocol) SendWarpSyncResponse(id iotago.CommitmentID, blockIDs iotago.BlockIDs, merkleProof *merklehasher.Proof[iotago.Identifier], to ...peer.ID) {
	serializer := p.apiProvider.APIForSlot(id.Index())

	p.network.Send(&nwmodels.Packet{Body: &nwmodels.Packet_WarpSyncResponse{
		WarpSyncResponse: &nwmodels.WarpSyncResponse{
			CommitmentId: lo.PanicOnErr(id.Bytes()),
			BlockIds:     lo.PanicOnErr(serializer.Encode(blockIDs)),
			MerkleProof:  lo.PanicOnErr(json.Marshal(merkleProof)),
		},
	}}, to...)
}

func (p *Protocol) handleWarpSyncRequest(commitmentIDBytes []byte, id peer.ID) {
	p.workerPool.Submit(func() {
		commitmentID, _, err := iotago.SlotIdentifierFromBytes(commitmentIDBytes)
		if err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitmentID in warp sync request"), id)

			return
		}

		p.Events.WarpSyncRequestReceived.Trigger(commitmentID, id)
	})
}

func (p *Protocol) handleWarpSyncResponse(commitmentIDBytes []byte, blockIDsBytes []byte, merkleProofBytes []byte, id peer.ID) {
	p.workerPool.Submit(func() {
		commitmentID, _, err := iotago.SlotIdentifierFromBytes(commitmentIDBytes)
		if err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize commitmentID in warp sync response"), id)

			return
		}

		var blockIDs iotago.BlockIDs
		if _, err = p.apiProvider.APIForSlot(commitmentID.Index()).Decode(blockIDsBytes, &blockIDs, serix.WithValidation()); err != nil {
			p.Events.Error.Trigger(ierrors.Wrap(err, "failed to deserialize block ids"), id)

			return
		}

		merkleProof := new(merklehasher.Proof[iotago.Identifier])
		if err = json.Unmarshal(merkleProofBytes, merkleProof); err != nil {
			p.Events.Error.Trigger(ierrors.Wrapf(err, "failed to deserialize merkle proof when receiving waprsync response for commitment %s", commitmentID), id)

			return
		}

		p.Events.WarpSyncResponseReceived.Trigger(commitmentID, blockIDs, merkleProof, id)
	})
}
