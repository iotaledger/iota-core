package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota.go/v4/api"

	iotago "github.com/iotaledger/iota.go/v4"
)

func (r *RequestHandler) GetCommitmentBySlot(slot iotago.SlotIndex) (*model.Commitment, error) {
	latest := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()

	if slot > latest.Slot() {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment is from a future slot (%d > %d)", slot, latest.Slot())
	}

	commitment, err := r.protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment, slot: %d, error: %w", slot, err)
	}

	return commitment, nil
}

// GetCommitmentByID returns the commitment for the given commitmentID. If commitmentID is empty, the latest commitment is returned.
func (r *RequestHandler) GetCommitmentByID(commitmentID iotago.CommitmentID) (*model.Commitment, error) {
	latest := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()
	if commitmentID == iotago.EmptyCommitmentID {
		return latest, nil
	}

	if commitmentID.Slot() > latest.Slot() {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment ID (%s) is from a future slot (%d > %d)", commitmentID, commitmentID.Slot(), latest.Slot())
	}

	commitment, err := r.protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentID.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
	}

	if commitment.ID() != commitmentID {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment in the store for slot %d does not match the given commitmentID (%s != %s)", commitmentID.Slot(), commitment.ID(), commitmentID)
	}

	return commitment, nil
}

func (r *RequestHandler) GetLatestCommitment() *model.Commitment {
	return r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()
}

func (r *RequestHandler) GetUTXOChanges(commitmentID iotago.CommitmentID) (*api.UTXOChangesResponse, error) {
	diffs, err := r.protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get slot diffs, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
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

func (r *RequestHandler) GetUTXOChangesFull(commitmentID iotago.CommitmentID) (*api.UTXOChangesFullResponse, error) {
	diffs, err := r.protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get slot diffs, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
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