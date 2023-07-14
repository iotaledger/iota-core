package coreapi

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
)

func getOutput(c echo.Context) (*utxoledger.Output, error) {
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
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, err
	}

	output, spent, err := deps.Protocol.MainEngineInstance().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, err
	}

	if spent != nil {
		return newSpentMetadataResponse(spent)
	}

	return newOutputMetadataResponse(output)
}

func newOutputMetadataResponse(output *utxoledger.Output) (*outputMetadataResponse, error) {
	latestCommitment := deps.Protocol.SyncManager.LatestCommitment()

	resp := &outputMetadataResponse{
		BlockID:            output.BlockID().ToHex(),
		TransactionID:      output.OutputID().TransactionID().ToHex(),
		OutputIndex:        output.OutputID().Index(),
		IsSpent:            false,
		LatestCommitmentID: latestCommitment.ID().ToHex(),
	}

	includedSlotIndex := output.SlotBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID().ToHex()
	}

	return resp, nil
}

func newSpentMetadataResponse(spent *utxoledger.Spent) (*outputMetadataResponse, error) {
	latestCommitment := deps.Protocol.SyncManager.LatestCommitment()

	resp := &outputMetadataResponse{
		BlockID:            spent.BlockID().ToHex(),
		TransactionID:      spent.OutputID().TransactionID().ToHex(),
		OutputIndex:        spent.OutputID().Index(),
		IsSpent:            true,
		TransactionIDSpent: spent.TransactionIDSpent().ToHex(),
		LatestCommitmentID: latestCommitment.ID().ToHex(),
	}

	includedSlotIndex := spent.Output().SlotBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID().ToHex()
	}

	spentSlotIndex := spent.SlotIndexSpent()
	if spentSlotIndex <= latestCommitment.Index() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.CommitmentIDSpent = spentCommitment.ID().ToHex()
	}

	return resp, nil
}
