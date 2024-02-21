package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (s *Server) ReadTransactionMetadata(_ context.Context, transactionID *inx.TransactionId) (*inx.TransactionMetadata, error) {
	return getINXTransactionMetadata(transactionID.Unwrap())
}

func getINXTransactionMetadata(transactionID iotago.TransactionID) (*inx.TransactionMetadata, error) {
	// TODO: wire this up with the tx retainer
	//nolint:staticcheck // TODO: remove this once we have the transaction retainer
	transactionMetadata := &api.TransactionMetadataResponse{TransactionID: transactionID}
	//nolint:staticcheck // TODO: remove this once we have the transaction retainer
	if transactionMetadata == nil {
		return nil, status.Errorf(codes.NotFound, "transaction not found")
	}

	return inx.WrapTransactionMetadata(transactionMetadata), nil
}
