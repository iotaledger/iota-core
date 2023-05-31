package coreapi

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

func getOutput(c echo.Context) (*ledgerstate.Output, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, err
	}

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID.UTXOInput())
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

	latestCommitment := deps.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment()

	unspent, err := deps.Protocol.MainEngineInstance().Ledger.IsOutputUnspent(outputID)
	if err != nil {
		return nil, err
	}

	if unspent {
		return newOutputMetadataResponse(outputID, latestCommitment)
	}

	return newSpentMetadataResponse(outputID, latestCommitment)
}

func newOutputMetadataResponse(outputID iotago.OutputID, latestCommitment *model.Commitment) (*outputMetadataResponse, error) {
	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID.UTXOInput())
	if err != nil {
		return nil, err
	}

	resp := &outputMetadataResponse{
		BlockID:            output.BlockID().ToHex(),
		TransactionID:      outputID.TransactionID().ToHex(),
		OutputIndex:        outputID.Index(),
		IsSpent:            false,
		LatestCommitmentID: latestCommitment.ID().ToHex(),
	}

	includedSlotIndex := output.SlotIndexBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID().ToHex()
	}

	return resp, nil
}

func newSpentMetadataResponse(outputID iotago.OutputID, latestCommitment *model.Commitment) (*outputMetadataResponse, error) {
	ledgerOutput, err := deps.Protocol.MainEngineInstance().Ledger.Spent(outputID)
	if err != nil {
		return nil, err
	}

	resp := &outputMetadataResponse{
		BlockID:            ledgerOutput.BlockID().ToHex(),
		TransactionID:      outputID.TransactionID().ToHex(),
		OutputIndex:        outputID.Index(),
		IsSpent:            true,
		TransactionIDSpent: ledgerOutput.TransactionIDSpent().ToHex(),
		LatestCommitmentID: latestCommitment.ID().ToHex(),
	}

	includedSlotIndex := ledgerOutput.Output().SlotIndexBooked()
	if includedSlotIndex <= latestCommitment.Index() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.IncludedCommitmentID = includedCommitment.ID().ToHex()
	}

	spentSlotIndex := ledgerOutput.SlotIndexSpent()
	if spentSlotIndex <= latestCommitment.Index() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, err
		}
		resp.CommitmentIDSpent = spentCommitment.ID().ToHex()
	}

	return resp, nil
}
