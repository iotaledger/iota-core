package protocol

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

func (p *Protocol) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs []iotago.BlockID, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	// TODO: CHECK WHICH CHAIN ASKED FOR THE WARPSYNC
	p.ChainManager.Chain(commitmentID)

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		acceptedBlocks.Add(blockID)
	}

	iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), iotago.Identifier{})

	p.CandidateEngineInstance().BlockRequester.StartTickers(blockIDs)
}

func (p *Protocol) processWarpSyncRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	committedSlot, err := p.MainEngineInstance().CommittedSlot(commitmentID.Index())
	if err != nil {
		return
	}

	commitment, err := committedSlot.Commitment()
	if err != nil || commitment.ID() != commitmentID {
		return
	}

	blockIDs, err := committedSlot.BlockIDs()
	if err != nil {
		return
	}

	roots, err := committedSlot.Roots()
	if err != nil {
		return
	}

	p.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.AttestationsProof(), src)
}
