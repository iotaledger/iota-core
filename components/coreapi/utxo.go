package coreapi

import (
	"errors"

	"github.com/labstack/echo/v4"

	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	iotago "github.com/iotaledger/iota.go/v4"
)

var ErrOutputNotCommitted = errors.New("the included slot index of the requested output is not committed yet")

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
	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, err
	}

	includedSlotIndex := output.SlotIndexBooked()
	if includedSlotIndex > latestCommitment.Index() {
		return nil, ErrOutputNotCommitted
	}

	includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
	if err != nil {
		return nil, err
	}

	return &outputMetadataResponse{
		BlockID:              output.BlockID().ToHex(),
		TransactionID:        outputID.TransactionID().ToHex(),
		OutputIndex:          outputID.Index(),
		IsSpent:              false,
		IncludedCommitmentID: includedCommitment.ID().ToHex(),
		LatestCommitmentID:   latestCommitment.ID().ToHex(),
	}, nil
}

func newSpentMetadataResponse(outputID iotago.OutputID, latestCommitment *model.Commitment) (*outputMetadataResponse, error) {
	ledgerOutput, err := deps.Protocol.MainEngineInstance().Ledger.Spent(outputID)
	if err != nil {
		return nil, err
	}

	includedSlotIndex := ledgerOutput.Output().SlotIndexBooked()
	spentSlotIndex := ledgerOutput.SlotIndexSpent()

	if includedSlotIndex > latestCommitment.Index() || spentSlotIndex > latestCommitment.Index() {
		return nil, ErrOutputNotCommitted
	}

	includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(includedSlotIndex)
	if err != nil {
		return nil, err
	}

	spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Permanent.Commitments().Load(spentSlotIndex)
	if err != nil {
		return nil, err
	}

	return &outputMetadataResponse{
		BlockID:              ledgerOutput.BlockID().ToHex(),
		TransactionID:        outputID.TransactionID().ToHex(),
		OutputIndex:          outputID.Index(),
		IsSpent:              true,
		TransactionIDSpent:   ledgerOutput.TransactionIDSpent().ToHex(),
		CommitmentIDSpent:    spentCommitment.ID().ToHex(),
		IncludedCommitmentID: includedCommitment.ID().ToHex(),
		LatestCommitmentID:   latestCommitment.ID().ToHex(),
	}, nil
}
