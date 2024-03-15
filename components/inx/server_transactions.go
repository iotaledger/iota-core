package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/retainer/txretainer"
)

func (s *Server) ReadTransactionMetadata(_ context.Context, transactionID *inx.TransactionId) (*inx.TransactionMetadata, error) {
	txID := transactionID.Unwrap()

	txMetadata, err := deps.Protocol.Engines.Main.Get().TxRetainer.TransactionMetadata(txID)
	if err != nil {
		if ierrors.Is(err, txretainer.ErrEntryNotFound) {
			return nil, status.Errorf(codes.NotFound, "transaction metadata not found: %s", txID.ToHex())
		}

		return nil, ierrors.Wrapf(err, "error when retrieving transaction metadata: %s", txID.ToHex())
	}

	return inx.WrapTransactionMetadata(txMetadata), nil
}
