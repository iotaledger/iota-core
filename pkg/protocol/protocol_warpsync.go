package protocol

import (
	"github.com/iotaledger/iota-core/pkg/network"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (p *Protocol) WarpSync(commitmentID iotago.CommitmentID, blockIDs []iotago.BlockID, _ network.PeerID) {
	// TODO: CHECK IF COMMITMENT IS VALID

	p.CandidateEngineInstance().BlockRequester.StartTickers(blockIDs)
}

func (p *Protocol) processWarpSyncRequest(commitmentID iotago.CommitmentID, src network.PeerID) {
	mainEngine := p.MainEngineInstance()

	if mainEngine.Storage.Settings().LatestCommitment().Index() < commitmentID.Index() {
		return
	}

	if commitment, err := mainEngine.Storage.Commitments().Load(commitmentID.Index()); err != nil {
		return
	} else if commitment.ID() != commitmentID {
		return
	}

	blockIDs := make([]iotago.BlockID, 0)
	if err := mainEngine.Storage.Blocks(commitmentID.Index()).ForEachBlockIDInSlot(func(blockID iotago.BlockID) error {
		blockIDs = append(blockIDs, blockID)

		return nil
	}); err != nil {
		return
	}

	p.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, src)
}
