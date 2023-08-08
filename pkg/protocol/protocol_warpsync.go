package protocol

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/network"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

func (p *Protocol) processWarpSyncResponse(commitmentID iotago.CommitmentID, blockIDs []iotago.BlockID, merkleProof *merklehasher.Proof[iotago.Identifier], _ network.PeerID) {
	acceptedBlocks := ads.NewSet[iotago.BlockID](mapdb.NewMapDB(), iotago.BlockID.Bytes, iotago.SlotIdentifierFromBytes)
	for _, blockID := range blockIDs {
		acceptedBlocks.Add(blockID)
	}

	iotago.VerifyProof(merkleProof, iotago.Identifier(acceptedBlocks.Root()), iotago.Identifier{})

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

	blockIDs, err := p.committedBlockIDs(mainEngine, commitmentID.Index())
	if err != nil {
		return
	}

	roots, err := p.roots(mainEngine, commitmentID.Index())
	if err != nil {
		return
	}

	p.networkProtocol.SendWarpSyncResponse(commitmentID, blockIDs, roots.AttestationsProof(), src)
}

func (p *Protocol) committedBlockIDs(engine *engine.Engine, slotIndex iotago.SlotIndex) (blockIDs []iotago.BlockID, err error) {
	if engine.Storage.Settings().LatestCommitment().Index() < slotIndex {
		return blockIDs, ierrors.Errorf("slot %d is not committed yet", slotIndex)
	}

	if err = engine.Storage.Blocks(slotIndex).ForEachBlockIDInSlot(func(blockID iotago.BlockID) error {
		blockIDs = append(blockIDs, blockID)

		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over blocks in slot %d", slotIndex)
	}

	return blockIDs, nil
}

func (p *Protocol) roots(engine *engine.Engine, slotIndex iotago.SlotIndex) (roots iotago.Roots, err error) {
	if engine.Storage.Settings().LatestCommitment().Index() < slotIndex {
		return roots, ierrors.Errorf("slot %d is not committed yet", slotIndex)
	}

	rootsStorage := engine.Storage.Roots(slotIndex)
	if rootsStorage == nil {
		return roots, ierrors.Errorf("no roots storage for slot %d", slotIndex)
	}

	rootsBytes, err := rootsStorage.Get(kvstore.Key{prunable.RootsKey})
	if err != nil {
		return roots, ierrors.Wrapf(err, "failed to load roots for slot %d", slotIndex)
	}

	_, err = p.APIForSlot(slotIndex).Decode(rootsBytes, &roots)
	if err != nil {
		return roots, ierrors.Wrapf(err, "failed to decode roots for slot %d", slotIndex)
	}

	return roots, nil
}
