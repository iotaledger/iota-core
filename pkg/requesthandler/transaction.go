package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"

	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (r *RequestHandler) BlockIDFromTransactionID(transactionID iotago.TransactionID) (iotago.BlockID, error) {
	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(transactionID, 0)

	output, spent, err := r.protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(echo.ErrInternalServerError, "failed to get output %s: %s", outputID.ToHex(), err)
	}

	if output != nil {
		return output.BlockID(), nil
	}

	return spent.BlockID(), nil
}

func (r *RequestHandler) TransactionMetadataByID(txID iotago.TransactionID) (*api.TransactionMetadataResponse, error) {
	txMetadata, err := r.protocol.Engines.Main.Get().TxRetainer.TransactionMetadata(txID)
	if err != nil {
		if ierrors.Is(err, txretainer.ErrEntryNotFound) {
			return nil, ierrors.WithMessagef(echo.ErrNotFound, "transaction metadata not found: %s", txID.ToHex())
		}

		return nil, ierrors.Join(echo.ErrInternalServerError, ierrors.Wrapf(err, "error when retrieving transaction metadata: %s", txID.ToHex()))
	}

	return txMetadata, nil
}
