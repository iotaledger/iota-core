package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func indexByCommitmentID(c echo.Context) (iotago.SlotIndex, error) {
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return iotago.SlotIndex(0), err
	}

	return commitmentID.Index(), nil
}

func getCommitmentDetails(index iotago.SlotIndex) (*iotago.Commitment, error) {
	commitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(index)
	if err != nil {
		return nil, err
	}

	return commitment.Commitment(), nil
}

func getUTXOChanges(index iotago.SlotIndex) (*apimodels.UTXOChangesResponse, error) {
	diffs, err := deps.Protocol.MainEngineInstance().Ledger.SlotDiffs(index)
	if err != nil {
		return nil, err
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
		Index:           index,
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}
