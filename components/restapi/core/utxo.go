package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func outputByID(c echo.Context) (*apimodels.OutputResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(restapipkg.ParameterOutputID))
	}

	output, err := deps.Protocol.MainEngineInstance().Ledger.Output(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	return &apimodels.OutputResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
	}, nil
}

func outputMetadataByID(c echo.Context) (*apimodels.OutputMetadata, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(restapipkg.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.MainEngineInstance().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	if spent != nil {
		return newSpentMetadataResponse(spent)
	}

	return newOutputMetadataResponse(output)
}

func outputWithMetadataByID(c echo.Context) (*apimodels.OutputWithMetadataResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(restapipkg.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.MainEngineInstance().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	if spent != nil {
		metadata, err := newSpentMetadataResponse(spent)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load spent output metadata: %s", err)
		}

		return &apimodels.OutputWithMetadataResponse{
			Output:        spent.Output().Output(),
			OutputIDProof: spent.Output().OutputIDProof(),
			Metadata:      metadata,
		}, nil
	}

	metadata, err := newOutputMetadataResponse(output)
	if err != nil {
		return nil, err
	}

	return &apimodels.OutputWithMetadataResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
		Metadata:      metadata,
	}, nil
}

func newOutputMetadataResponse(output *utxoledger.Output) (*apimodels.OutputMetadata, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadata{
		BlockID:            output.BlockID(),
		TransactionID:      output.OutputID().TransactionID(),
		OutputIndex:        output.OutputID().Index(),
		IsSpent:            false,
		LatestCommitmentID: latestCommitment.ID(),
	}

	includedSlotIndex := output.SlotBooked()
	if includedSlotIndex <= latestCommitment.Slot() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", includedSlotIndex, err)
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	return resp, nil
}

func newSpentMetadataResponse(spent *utxoledger.Spent) (*apimodels.OutputMetadata, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadata{
		BlockID:            spent.BlockID(),
		TransactionID:      spent.OutputID().TransactionID(),
		OutputIndex:        spent.OutputID().Index(),
		IsSpent:            true,
		TransactionIDSpent: spent.TransactionIDSpent(),
		LatestCommitmentID: latestCommitment.ID(),
	}

	includedSlotIndex := spent.Output().SlotBooked()
	if includedSlotIndex <= latestCommitment.Slot() {
		includedCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(includedSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", includedSlotIndex, err)
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	spentSlotIndex := spent.SlotSpent()
	if spentSlotIndex <= latestCommitment.Slot() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", spentSlotIndex, err)
		}
		resp.CommitmentIDSpent = spentCommitment.ID()
	}

	return resp, nil
}
