package requesthandler

import (
	"github.com/labstack/echo/v4"

	"github.com/iotaledger/hive.go/ierrors"

	iotago "github.com/iotaledger/iota.go/v4"
)

func (r *RequestHandler) BlockIDFromTransactionID(transactionID iotago.TransactionID) (iotago.BlockID, error) {
	// Get the first output of that transaction (using index 0)
	outputID := iotago.OutputIDFromTransactionIDAndIndex(transactionID, 0)

	output, spent, err := r.protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(echo.ErrNotFound, "output %s of transaction %s not found: %s", transactionID.ToHex(), outputID.ToHex(), err)
	}

	if output != nil {
		return output.BlockID(), nil
	}

	return spent.BlockID(), nil
}
