package coreapi

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
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
	latestCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
	includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(output.BlockID().Index())
	if err != nil {
		return nil, err
	}

	unspent, err := deps.Protocol.MainEngineInstance().Ledger.IsOutputUnspent(outputID)
	if err != nil {
		return nil, err
	}

	if unspent {
		return newOutputMetadataResponse(output, latestCommitment, includedCommitment), nil
	}

	ledgerOutput, err := deps.Protocol.MainEngineInstance().Ledger.Spent(outputID)
	if err != nil {
		return nil, err
	}

	spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(ledgerOutput.SlotIndexSpent())
	if err != nil {
		return nil, err
	}

	return newSpentMetadataResponse(ledgerOutput, latestCommitment, includedCommitment, spentCommitment), nil
}

func newOutputMetadataResponse(output *ledgerstate.Output, latestCommitment, includedCommitment *model.Commitment) *outputMetadataResponse {
	outputID := output.OutputID()
	return &outputMetadataResponse{
		BlockID:              output.BlockID().ToHex(),
		TransactionID:        outputID.TransactionID().ToHex(),
		OutputIndex:          outputID.Index(),
		IsSpent:              false,
		IncludedCommitmentID: includedCommitment.ID().ToHex(),
		LatestCommitmentID:   latestCommitment.ID().ToHex(),
	}
}

func newSpentMetadataResponse(output *ledgerstate.Spent, latestCommitment, includedCommitment, spentCommitment *model.Commitment) *outputMetadataResponse {
	outputID := output.OutputID()
	return &outputMetadataResponse{
		BlockID:              output.BlockID().ToHex(),
		TransactionID:        outputID.TransactionID().ToHex(),
		OutputIndex:          outputID.Index(),
		IsSpent:              true,
		TransactionIDSpent:   output.TransactionIDSpent().ToHex(),
		CommitmentIDSpent:    spentCommitment.ID().ToHex(),
		IncludedCommitmentID: includedCommitment.ID().ToHex(),
		LatestCommitmentID:   latestCommitment.ID().ToHex(),
	}
}
