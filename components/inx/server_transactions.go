package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) ReadTransactionMetadata(_ context.Context, transactionID *inx.TransactionId) (*inx.TransactionMetadata, error) {
	return getINXTransactionMetadata(transactionID.Unwrap())
}

func getINXTransactionMetadata(transactionID iotago.TransactionID) (*inx.TransactionMetadata, error) {

	blockIDFromTransactionID := func(transactionID iotago.TransactionID) (iotago.BlockID, error) {
		// Get the first output of that transaction (using index 0)
		outputID := iotago.OutputIDFromTransactionIDAndIndex(transactionID, 0)

		output, spent, err := deps.Protocol.Engines.Main.Get().Ledger.OutputOrSpent(outputID)
		if err != nil {
			return iotago.EmptyBlockID, status.Errorf(codes.Internal, "failed to get output %s: %s", outputID.ToHex(), err)
		}

		if output != nil {
			return output.BlockID(), nil
		}

		return spent.BlockID(), nil
	}

	blockID, err := blockIDFromTransactionID(transactionID)
	if err != nil {
		return nil, err
	}

	blockMetadata, err := deps.Protocol.Engines.Main.Get().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get block metadata %s: %s", blockID.ToHex(), err)
	}

	transactionMetadata := blockMetadata.TransactionMetadataResponse()
	if transactionMetadata == nil {
		return nil, status.Errorf(codes.NotFound, "transaction not found")
	}

	/*
		// TODO: change the retainer to store the transaction metadata
		transactionMetadata, err := deps.Protocol.MainEngineInstance().Retainer.TransactionMetadata(transactionID)
		if err != nil {
			return nil, ierrors.Errorf("failed to get transaction metadata: %v", err)
		}
	*/
	return inx.WrapTransactionMetadata(transactionMetadata), nil
}
