package protocol

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/chainmanager"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

func (p *Protocol) targetEngine(commitment *chainmanager.ChainCommitment) *engine.Engine {
	chain := commitment.Chain()
	if chain == nil {
		return nil
	}

	if mainEngineInstance := p.MainEngineInstance(); mainEngineInstance.ChainID() == chain.ForkingPoint.Commitment().ID() {
		return mainEngineInstance
	}

	if candidateEngineInstance := p.CandidateEngineInstance(); candidateEngineInstance != nil && candidateEngineInstance.ChainID() == chain.ForkingPoint.Commitment().ID() {
		return candidateEngineInstance
	}

	return nil
}

func (p *Protocol) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs []iotago.BlockID, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	commitment, exists := p.ChainManager.Commitment(commitmentID)
	if !exists {
		return
	}

	targetEngine := p.targetEngine(commitment)
	if targetEngine == nil {
		return
	}

	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		_ = acceptedBlocks.Add(blockID) // a mapdb can newer return an error
	}

	if !iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), commitment.Commitment().RootsID()) {
		return
	}

	for _, blockID := range blockIDs {
		targetEngine.BlockDAG.GetOrRequestBlock(blockID)
	}
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
