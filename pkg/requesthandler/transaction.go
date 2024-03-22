package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"

	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) BlockIDFromTransactionID(transactionID iotago.TransactionID) (iotago.BlockID, error) {
	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(transactionID, 0)

	output, spent, err := r.protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return iotago.EmptyBlockID, ierrors.WithMessagef(echo.ErrNotFound, "output %s not found", outputID.ToHex())
		}

		return iotago.EmptyBlockID, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to get output %s: %w", outputID.ToHex(), err)
	}

	if output != nil {
		return output.BlockID(), nil
	}

	return spent.BlockID(), nil
}

func (r *RequestHandler) BlockFromTransactionID(transactionID iotago.TransactionID) (*iotago.Block, error) {
	blockID, err := r.BlockIDFromTransactionID(transactionID)
	if err != nil {
		return nil, err
	}

	return r.BlockFromBlockID(blockID)
}

func (r *RequestHandler) BlockMetadataFromTransactionID(txID iotago.TransactionID) (*api.BlockMetadataResponse, error) {
	blockID, err := r.BlockIDFromTransactionID(txID)
	if err != nil {
		return nil, ierrors.WithMessagef(echo.ErrNotFound, "failed to get block ID from transaction ID: %v", err)
	}

	return r.BlockMetadataFromBlockID(blockID)
}

func (r *RequestHandler) TransactionMetadataFromTransactionID(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error) {
	txMetadata, err := r.protocol.Engines.Main.Get().TxRetainer.TransactionMetadata(txID)
	if err != nil {
		if ierrors.Is(err, txretainer.ErrEntryNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "transaction metadata for transaction %s not found", txID.ToHex())
		}

		return nil, ierrors.WithMessagef(echo.ErrInternalServerError, "failed to retrieve transaction metadata for transaction %s: %w", txID.ToHex(), err)
	}

	return txMetadata, nil
}
