package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) OutputFromOutputID(outputID iotago.OutputID) (*api.OutputResponse, error) {
	output, err := r.protocol.Engines.Main.Get().Ledger.Output(outputID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "output %s not found in the Ledger", outputID.ToHex())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get output %s from the ledger: %w", outputID.ToHex(), err)
	}

	return &api.OutputResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
	}, nil
}

func (r *RequestHandler) OutputMetadataFromOutputID(outputID iotago.OutputID) (*api.OutputMetadata, error) {
	output, spent, err := r.protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "output %s not found in the Ledger", outputID.ToHex())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %w", outputID.ToHex(), err)
	}

	if spent != nil {
		return r.newSpentMetadataResponse(spent)
	}

	return r.newOutputMetadataResponse(output)
}

func (r *RequestHandler) OutputWithMetadataFromOutputID(outputID iotago.OutputID) (*api.OutputWithMetadataResponse, error) {
	output, spent, err := r.protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "output %s not found in the Ledger", outputID.ToHex())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get output %s from the Ledger: %w", outputID.ToHex(), err)
	}

	if spent != nil {
		metadata, err := r.newSpentMetadataResponse(spent)
		if err != nil {
			return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to load spent output metadata: %w", err)
		}

		return &api.OutputWithMetadataResponse{
			Output:        spent.Output().Output(),
			OutputIDProof: spent.Output().OutputIDProof(),
			Metadata:      metadata,
		}, nil
	}

	metadata, err := r.newOutputMetadataResponse(output)
	if err != nil {
		return nil, err
	}

	return &api.OutputWithMetadataResponse{
		Output:        output.Output(),
		OutputIDProof: output.OutputIDProof(),
		Metadata:      metadata,
	}, nil
}

func (r *RequestHandler) newOutputMetadataResponse(output *utxoledger.Output) (*api.OutputMetadata, error) {
	latestCommitment := r.protocol.Engines.Main.Get().SyncManager.LatestCommitment()

	includedSlot := output.SlotBooked()
	includedCommitmentID := iotago.EmptyCommitmentID

	if includedSlot <= latestCommitment.Slot() &&
		includedSlot >= r.protocol.Engines.Main.Get().CommittedAPI().ProtocolParameters().GenesisSlot() {
		includedCommitment, err := r.protocol.Engines.Main.Get().Storage.Commitments().Load(includedSlot)
		if err != nil {
			return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to load commitment with index %d: %s", includedSlot, err)
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

func (r *RequestHandler) newSpentMetadataResponse(spent *utxoledger.Spent) (*api.OutputMetadata, error) {
	newOutputMetadataResponse, err := r.newOutputMetadataResponse(spent.Output())
	if err != nil {
		return nil, err
	}

	spentSlot := spent.SlotSpent()
	spentCommitmentID := iotago.EmptyCommitmentID

	if spentSlot <= newOutputMetadataResponse.LatestCommitmentID.Slot() &&
		spentSlot >= r.protocol.Engines.Main.Get().CommittedAPI().ProtocolParameters().GenesisSlot() {
		spentCommitment, err := r.protocol.Engines.Main.Get().Storage.Commitments().Load(spentSlot)
		if err != nil {
			return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to load commitment with index %d: %w", spentSlot, err)
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
