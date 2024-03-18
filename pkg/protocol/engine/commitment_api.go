package engine

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/merklehasher"
)

// CommitmentAPI is a wrapper for the Engine that provides access to the data of a committed slot.
type CommitmentAPI struct {
	// engine is the Engine that is used to access the data.
	engine *Engine

	// CommitmentID is the index of the slot that is accessed.
	CommitmentID iotago.CommitmentID
}

// NewCommitmentAPI creates a new CommitmentAPI.
func NewCommitmentAPI(engine *Engine, commitmentID iotago.CommitmentID) *CommitmentAPI {
	return &CommitmentAPI{
		engine:       engine,
		CommitmentID: commitmentID,
	}
}

// Commitment returns the commitment of the slot.
func (c *CommitmentAPI) Commitment() (commitment *model.Commitment, err error) {
	if commitment, err = c.engine.Storage.Commitments().Load(c.CommitmentID.Slot()); err != nil {
		return nil, ierrors.Wrapf(err, "failed to load commitment for slot %d", c.CommitmentID)
	}

	if commitment.ID() != c.CommitmentID {
		return nil, ierrors.Errorf("commitment in the store does not match the given commitmentID (%s != %s)", commitment.ID(), c.CommitmentID)
	}

	return commitment, nil
}

// Attestations returns the commitment, attestations and the merkle proof of the slot.
func (c *CommitmentAPI) Attestations() (commitment *model.Commitment, attestations []*iotago.Attestation, merkleProof *merklehasher.Proof[iotago.Identifier], err error) {
	commitment, err = c.Commitment()
	if err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "failed to load commitment")
	}

	if attestations, err = c.engine.Attestations.Get(c.CommitmentID.Slot()); err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "failed to load attestations")
	}

	rootsStorage, err := c.engine.Storage.Roots(c.CommitmentID.Slot())
	if err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "failed to load roots storage")
	}

	roots, exists, err := rootsStorage.Load(c.CommitmentID)
	if err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "failed to load roots")
	} else if !exists {
		return nil, nil, nil, ierrors.New("roots not found")
	}

	return commitment, attestations, roots.AttestationsProof(), nil
}

// Mutations returns all accepted block IDs, the tangle proof, all accepted transaction IDs and the ledger state
// mutation proof of the slot.
func (c *CommitmentAPI) Mutations() (acceptedBlocksBySlotCommitment map[iotago.CommitmentID]iotago.BlockIDs, acceptedBlocksProof *merklehasher.Proof[iotago.Identifier], acceptedTransactionIDs iotago.TransactionIDs, acceptedTransactionsProof *merklehasher.Proof[iotago.Identifier], err error) {
	if acceptedBlocksBySlotCommitment, err = c.BlocksIDsBySlotCommitmentID(); err != nil {
		return nil, nil, nil, nil, ierrors.Wrap(err, "failed to get block ids")
	}

	roots, err := c.Roots()
	if err != nil {
		return nil, nil, nil, nil, ierrors.Wrap(err, "failed to get roots")
	}

	acceptedTransactionIDs, err = c.TransactionIDs()
	if err != nil {
		return nil, nil, nil, nil, ierrors.Wrap(err, "failed to get transaction ids")
	}

	return acceptedBlocksBySlotCommitment, roots.TangleProof(), acceptedTransactionIDs, roots.MutationProof(), nil
}

// Roots returns the roots of the slot.
func (c *CommitmentAPI) Roots() (committedRoots *iotago.Roots, err error) {
	if c.engine.SyncManager.LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID.Slot())
	}

	rootsStorage, err := c.engine.Storage.Roots(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("no roots storage for slot %d", c.CommitmentID.Slot())
	}

	roots, _, err := rootsStorage.Load(c.CommitmentID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to load roots for slot %d", c.CommitmentID.Slot())
	} else if roots == nil {
		return nil, ierrors.Errorf("roots for slot %d are not known, yet", c.CommitmentID.Slot())
	}

	return roots, nil
}

// BlocksIDsBySlotCommitmentID returns the accepted block IDs of the slot grouped by their SlotCommitmentID.
func (c *CommitmentAPI) BlocksIDsBySlotCommitmentID() (map[iotago.CommitmentID]iotago.BlockIDs, error) {
	if c.engine.SyncManager.LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID.Slot())
	}

	store, err := c.engine.Storage.Blocks(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("failed to get block store of slot index %d", c.CommitmentID.Slot())
	}

	blockIDsBySlotCommitmentID := make(map[iotago.CommitmentID]iotago.BlockIDs)
	if err := store.ForEachBlockInSlot(func(block *model.Block) error {
		blockIDsBySlotCommitmentID[block.SlotCommitmentID()] = append(blockIDsBySlotCommitmentID[block.SlotCommitmentID()], block.ID())
		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over blocks of slot %d", c.CommitmentID.Slot())
	}

	return blockIDsBySlotCommitmentID, nil
}

func (c *CommitmentAPI) TransactionIDs() (iotago.TransactionIDs, error) {
	if c.engine.SyncManager.LatestCommitment().Slot() < c.CommitmentID.Slot() {
		return nil, ierrors.Errorf("slot %d is not committed yet", c.CommitmentID.Slot())
	}

	store, err := c.engine.Storage.Mutations(c.CommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Errorf("failed to get mutations of slot index %d", c.CommitmentID.Slot())
	}

	set := ads.NewSet[iotago.Identifier](
		store,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.TransactionID.Bytes,
		iotago.TransactionIDFromBytes,
	)
	transactionIDs := make(iotago.TransactionIDs, 0, set.Size())

	if err = set.Stream(func(txID iotago.TransactionID) error {
		transactionIDs = append(transactionIDs, txID)
		return nil
	}); err != nil {
		return nil, ierrors.Wrapf(err, "failed to iterate over mutations of slot %d", c.CommitmentID.Slot())
	}

	return transactionIDs, nil
}
