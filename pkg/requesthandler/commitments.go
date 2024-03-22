package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota.go/v4/api"

	iotago "github.com/iotaledger/iota.go/v4"
)

func (r *RequestHandler) GetCommitmentBySlot(slot iotago.SlotIndex) (*model.Commitment, error) {
	latest := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()

	if slot > latest.Slot() {
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "commitment is from a future slot (%d > %d)", slot, latest.Slot())
	}

	commitment, err := r.protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
	if err != nil {
		if ierrors.Is(err, permanent.ErrCommitmentBeforeGenesis) {
			return nil, ierrors.Chain(httpserver.ErrInvalidParameter, err)
		}
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "commitment not found, slot: %d", slot)
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to load commitment, slot: %d, error: %w", slot, err)
	}

	return commitment, nil
}

// GetCommitmentByID returns the commitment for the given commitmentID. If commitmentID is empty, the latest commitment is returned.
func (r *RequestHandler) GetCommitmentByID(commitmentID iotago.CommitmentID) (*model.Commitment, error) {
	if commitmentID == iotago.EmptyCommitmentID {
		// this returns the latest commitment in the case that the commitmentID is empty
		return r.protocol.Engines.Main.Get().SyncManager.LatestCommitment(), nil
	}

	commitment, err := r.GetCommitmentBySlot(commitmentID.Slot())
	if err != nil {
		return nil, err
	}

	if commitment.ID() != commitmentID {
		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "commitment in the store for slot %d does not match the given commitmentID (%s != %s)", commitmentID.Slot(), commitment.ID(), commitmentID)
	}

	return commitment, nil
}

func (r *RequestHandler) GetLatestCommitment() *model.Commitment {
	return r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()
}

// GetUTXOChangesByCommitmentID returns the UTXO changes for the given commitmentID. If commitmentID is empty, the latest commitment is used.
func (r *RequestHandler) GetUTXOChangesByCommitmentID(commitmentID iotago.CommitmentID) (*api.UTXOChangesResponse, error) {
	if commitmentID == iotago.EmptyCommitmentID {
		// this returns the latest commitment in the case that the commitmentID is empty
		commitment, err := r.GetCommitmentByID(commitmentID)
		if err != nil {
			return nil, err
		}
		commitmentID = commitment.ID()
	}

	return r.getUTXOChanges(commitmentID)
}

// GetUTXOChangesBySlot returns the UTXO changes for the given slot.
func (r *RequestHandler) GetUTXOChangesBySlot(slot iotago.SlotIndex) (*api.UTXOChangesResponse, error) {
	commitment, err := r.GetCommitmentBySlot(slot)
	if err != nil {
		return nil, err
	}

	return r.getUTXOChanges(commitment.ID())
}

// GetUTXOChangesFullByCommitmentID returns the UTXO changes for the given commitmentID. If commitmentID is empty, the latest commitment is used.
func (r *RequestHandler) GetUTXOChangesFullByCommitmentID(commitmentID iotago.CommitmentID) (*api.UTXOChangesFullResponse, error) {
	if commitmentID == iotago.EmptyCommitmentID {
		// this returns the latest commitment in the case that the commitmentID is empty
		commitment, err := r.GetCommitmentByID(commitmentID)
		if err != nil {
			return nil, err
		}
		commitmentID = commitment.ID()
	}

	return r.getUTXOChangesFull(commitmentID)
}

// GetUTXOChangesFullBySlot returns the UTXO changes for the given slot.
func (r *RequestHandler) GetUTXOChangesFullBySlot(slot iotago.SlotIndex) (*api.UTXOChangesFullResponse, error) {
	commitment, err := r.GetCommitmentBySlot(slot)
	if err != nil {
		return nil, err
	}

	return r.getUTXOChangesFull(commitment.ID())
}

func (r *RequestHandler) getUTXOChanges(commitmentID iotago.CommitmentID) (*api.UTXOChangesResponse, error) {
	diffs, err := r.protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "slot diffs not found, commitmentID: %s, slot: %d", commitmentID, commitmentID.Slot())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get slot diffs, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
	}

	createdOutputs := make(iotago.OutputIDs, len(diffs.Outputs))
	consumedOutputs := make(iotago.OutputIDs, len(diffs.Spents))

	for i, output := range diffs.Outputs {
		createdOutputs[i] = output.OutputID()
	}

	for i, output := range diffs.Spents {
		consumedOutputs[i] = output.OutputID()
	}

	return &api.UTXOChangesResponse{
		CommitmentID:    commitmentID,
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}

func (r *RequestHandler) getUTXOChangesFull(commitmentID iotago.CommitmentID) (*api.UTXOChangesFullResponse, error) {
	diffs, err := r.protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "slot diffs not found, commitmentID: %s, slot: %d", commitmentID, commitmentID.Slot())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get slot diffs, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
	}

	createdOutputs := make([]*api.OutputWithID, len(diffs.Outputs))
	consumedOutputs := make([]*api.OutputWithID, len(diffs.Spents))

	for i, output := range diffs.Outputs {
		createdOutputs[i] = &api.OutputWithID{
			OutputID: output.OutputID(),
			Output:   output.Output(),
		}
	}

	for i, output := range diffs.Spents {
		consumedOutputs[i] = &api.OutputWithID{
			OutputID: output.OutputID(),
			Output:   output.Output().Output(),
		}
	}

	return &api.UTXOChangesFullResponse{
		CommitmentID:    commitmentID,
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}

func (r *RequestHandler) CommittedAPI() iotago.API {
	return r.protocol.CommittedAPI()
}

func (r *RequestHandler) LatestAPI() iotago.API {
	return r.protocol.LatestAPI()
}
