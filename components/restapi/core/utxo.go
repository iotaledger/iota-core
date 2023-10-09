package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	restapipkg "github.com/iotaledger/iota-core/pkg/restapi"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

func getOutput(c echo.Context) (*utxoledger.Output, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID param: %s", c.Param(restapipkg.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.MainEngineInstance().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get output: %s from the Ledger", outputID.String())
	}

	if spent != nil {
		output = spent.Output()
	}

	return output, nil
}

func getOutputMetadata(c echo.Context) (*apimodels.OutputMetadataResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, restapipkg.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID param: %s", c.Param(restapipkg.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.MainEngineInstance().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to get output: %s from the Ledger", outputID.String())
	}

	if spent != nil {
		return newSpentMetadataResponse(spent)
	}

	return newOutputMetadataResponse(output)
}

func newOutputMetadataResponse(output *utxoledger.Output) (*apimodels.OutputMetadataResponse, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadataResponse{
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
			return nil, ierrors.Wrapf(err, "failed to load commitment with index: %d", includedSlotIndex)
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	return resp, nil
}

func newSpentMetadataResponse(spent *utxoledger.Spent) (*apimodels.OutputMetadataResponse, error) {
	latestCommitment := deps.Protocol.MainEngineInstance().SyncManager.LatestCommitment()

	resp := &apimodels.OutputMetadataResponse{
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
			return nil, ierrors.Wrapf(err, "failed to load commitment with index: %d", includedSlotIndex)
		}
		resp.IncludedCommitmentID = includedCommitment.ID()
	}

	spentSlotIndex := spent.SlotIndexSpent()
	if spentSlotIndex <= latestCommitment.Slot() {
		spentCommitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(spentSlotIndex)
		if err != nil {
			return nil, ierrors.Wrapf(err, "failed to load commitment with index: %d", spentSlotIndex)
		}
		resp.CommitmentIDSpent = spentCommitment.ID()
	}

	return resp, nil
}
