package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func getCommitmentBySlot(slot iotago.SlotIndex, latestCommitment ...*model.Commitment) (*model.Commitment, error) {
	var latest *model.Commitment
	if len(latestCommitment) > 0 {
		latest = latestCommitment[0]
	} else {
		latest = deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment()
	}

	if slot > latest.Slot() {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment is from a future slot (%d > %d)", slot, latest.Slot())
	}

	commitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(slot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment, slot: %d, error: %w", slot, err)
	}

	return commitment, nil
}

func getCommitmentByID(commitmentID iotago.CommitmentID, latestCommitment ...*model.Commitment) (*model.Commitment, error) {
	var latest *model.Commitment
	if len(latestCommitment) > 0 {
		latest = latestCommitment[0]
	} else {
		latest = deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment()
	}

	if commitmentID.Slot() > latest.Slot() {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment ID (%s) is from a future slot (%d > %d)", commitmentID, commitmentID.Slot(), latest.Slot())
	}

	commitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentID.Slot())
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment, commitmentID: %s, slot: %d, error: %w", commitmentID, commitmentID.Slot(), err)
	}

	if commitment.ID() != commitmentID {
		return nil, ierrors.Wrapf(echo.ErrBadRequest, "commitment in the store for slot %d does not match the given commitmentID (%s != %s)", commitmentID.Slot(), commitment.ID(), commitmentID)
	}

	return commitment, nil
}

func getUTXOChanges(commitmentID iotago.CommitmentID) (*apimodels.UTXOChangesResponse, error) {
	diffs, err := deps.Protocol.Engines.Main.Get().Ledger.SlotDiffs(commitmentID.Slot())
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

	return &apimodels.UTXOChangesResponse{
		CommitmentID:    commitmentID,
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}
