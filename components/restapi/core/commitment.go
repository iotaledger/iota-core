package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func indexByCommitmentID(c echo.Context) (iotago.SlotIndex, error) {
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return iotago.SlotIndex(0), ierrors.Wrapf(err, "failed to parse commitment ID %s", c.Param(restapipkg.ParameterCommitmentID))
	}

	return commitmentID.Slot(), nil
}

func getCommitmentDetails(index iotago.SlotIndex) (*iotago.Commitment, error) {
	commitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(index)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment %d: %s", index, err)
	}

	return commitment.Commitment(), nil
}

func getUTXOChanges(slot iotago.SlotIndex) (*apimodels.UTXOChangesResponse, error) {
	diffs, err := deps.Protocol.Engines.Main.Get().Ledger.SlotDiffs(slot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get slot diffs %d: %s", slot, err)
	}

	commitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(diffs.Slot)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment %d: %s", diffs.Slot, err)
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
		CommitmentID:    commitment.ID(),
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}
