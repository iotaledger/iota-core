package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
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

func getOutputMetadata(c echo.Context) (*apimodels.OutputMetadataResponse, error) {
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

func newOutputMetadataResponse(output *utxoledger.Output) (*apimodels.OutputMetadataResponse, error) {
	latestCommitment := deps.Protocol.SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadataResponse{
		BlockID:            output.BlockID(),
		TransactionID:      output.OutputID().TransactionID(),
		OutputIndex:        output.OutputID().Index(),
		IsSpent:            false,
		LatestCommitmentID: latestCommitment.ID(),
	}

	includedSlotIndex := output.SlotBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	return resp, nil
}

func newSpentMetadataResponse(spent *utxoledger.Spent) (*apimodels.OutputMetadataResponse, error) {
	latestCommitment := deps.Protocol.SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadataResponse{
		BlockID:            spent.BlockID(),
		TransactionID:      spent.OutputID().TransactionID(),
		OutputIndex:        spent.OutputID().Index(),
		IsSpent:            true,
		TransactionIDSpent: spent.TransactionIDSpent(),
		LatestCommitmentID: latestCommitment.ID(),
	}

	includedSlotIndex := spent.Output().SlotBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	spentSlotIndex := spent.SlotIndexSpent()
	if spentSlotIndex <= latestCommitment.Index() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.CommitmentIDSpent = spentCommitment.ID()
	}

	return resp, nil
}
