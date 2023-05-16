package coreapi

import (
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/labstack/echo/v4"
)

func getOutput(c echo.Context) (*ledgerstate.Output, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, err
	}

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func getOutputMetadata(c echo.Context) (*outputMetadataResponse, error) {
	output, err := getOutput(c)
	if err != nil {
		return nil, err
	}
	outputID := output.OutputID()
	slotCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
	spent, err := deps.Protocol.MainEngineInstance().Ledger.IsOutputSpent(outputID)
	if err != nil {
		return nil, err
	}

	// TODO: CommitmentIDSpent,TransactionIDSpent, CommitmentIDConfirmed
	return &outputMetadataResponse{
		BlockID:            output.BlockID().ToHex(),
		TransactionID:      outputID.TransactionID().ToHex(),
		OutputIndex:        outputID.Index(),
		IsSpent:            spent,
		LatestCommitmentID: slotCommitment.ID().ToHex(),
	}, nil
}
