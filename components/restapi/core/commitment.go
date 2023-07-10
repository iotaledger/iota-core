package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

func indexByCommitmentID(c echo.Context) (iotago.SlotIndex, error) {
	commitmentID, err := httpserver.ParseCommitmentIDParam(c, restapipkg.ParameterCommitmentID)
	if err != nil {
		return iotago.SlotIndex(0), err
	}

	return commitmentID.Index(), nil
}

func getCommitment(index iotago.SlotIndex) (*commitmentInfoResponse, error) {
	commitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(index)
	if err != nil {
		return nil, err
	}

	return &commitmentInfoResponse{
		Index:            commitment.Index(),
		PrevID:           commitment.PrevID().ToHex(),
		RootsID:          commitment.RootsID().ToHex(),
		CumulativeWeight: commitment.CumulativeWeight(),
	}, nil
}

func getSlotUTXOChanges(index iotago.SlotIndex) (*slotUTXOResponse, error) {
	diffs, err := deps.Protocol.MainEngineInstance().Ledger.StateDiffs(index)
	if err != nil {
		return nil, err
	}

	createdOutputs := make([]string, len(diffs.Outputs))
	consumedOutputs := make([]string, len(diffs.Spents))

	for i, output := range diffs.Outputs {
		createdOutputs[i] = output.OutputID().ToHex()
	}

	for i, output := range diffs.Spents {
		consumedOutputs[i] = output.OutputID().ToHex()
	}

	return &slotUTXOResponse{
		Index:           index,
		CreatedOutputs:  createdOutputs,
		ConsumedOutputs: consumedOutputs,
	}, nil
}
