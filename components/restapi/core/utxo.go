package core

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/inx-app/pkg/httpserver"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func outputByID(c echo.Context) (*api.OutputResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	output, err := deps.Protocol.Engines.Main.Get().Ledger.Output(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	return &api.OutputResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
	}, nil
}

func outputMetadataByID(c echo.Context) (*api.OutputMetadata, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	if spent != nil {
		return newSpentMetadataResponse(spent)
	}

	return newOutputMetadataResponse(output)
}

func outputWithMetadataByID(c echo.Context) (*api.OutputWithMetadataResponse, error) {
	outputID, err := httpserver.ParseOutputIDParam(c, api.ParameterOutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to parse output ID %s", c.Param(api.ParameterOutputID))
	}

	output, spent, err := deps.Protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %s", outputID.ToHex(), err)
	}

	if spent != nil {
		metadata, err := newSpentMetadataResponse(spent)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load spent output metadata: %s", err)
		}

		return &api.OutputWithMetadataResponse{
			Output:        spent.Output().Output(),
			OutputIDProof: spent.Output().OutputIDProof(),
			Metadata:      metadata,
		}, nil
	}

	metadata, err := newOutputMetadataResponse(output)
	if err != nil {
		return nil, err
	}

	return &api.OutputWithMetadataResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
		Metadata:      metadata,
	}, nil
}

func newOutputMetadataResponse(output *utxoledger.Output) (*api.OutputMetadata, error) {
	latestCommitment := deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment()

	includedSlot := output.SlotBooked()
	includedCommitmentID := iotago.EmptyCommitmentID

	if includedSlot <= latestCommitment.Slot() &&
		includedSlot >= deps.Protocol.Engines.Main.Get().CommittedAPI().ProtocolParameters().GenesisSlot() {
		includedCommitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(includedSlot)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", includedSlot, err)
		}
		includedCommitmentID = includedCommitment.ID()
	}

	return &api.OutputMetadata{
		OutputID: output.OutputID(),
		BlockID:  output.BlockID(),
		Included: &api.OutputInclusionMetadata{
			Slot:          includedSlot,
			TransactionID: output.OutputID().TransactionID(),
			CommitmentID:  includedCommitmentID,
		},
		LatestCommitmentID: latestCommitment.ID(),
	}, nil
}

func newSpentMetadataResponse(spent *utxoledger.Spent) (*api.OutputMetadata, error) {
	newOutputMetadataResponse, err := newOutputMetadataResponse(spent.Output())
	if err != nil {
		return nil, err
	}

	spentSlot := spent.SlotSpent()
	spentCommitmentID := iotago.EmptyCommitmentID

	if spentSlot <= newOutputMetadataResponse.LatestCommitmentID.Slot() &&
		spentSlot >= deps.Protocol.Engines.Main.Get().CommittedAPI().ProtocolParameters().GenesisSlot() {
		spentCommitment, err := deps.Protocol.Engines.Main.Get().Storage.Commitments().Load(spentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", spentSlot, err)
		}
		spentCommitmentID = spentCommitment.ID()
	}

	newOutputMetadataResponse.Spent = &api.OutputConsumptionMetadata{
		Slot:          spentSlot,
		TransactionID: spent.TransactionIDSpent(),
		CommitmentID:  spentCommitmentID,
	}

	return newOutputMetadataResponse, nil
}
