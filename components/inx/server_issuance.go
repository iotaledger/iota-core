package inx

import (
	"context"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	inx "github.com/iotaledger/inx/go"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) RequestTips(_ context.Context, req *inx.TipsRequest) (*inx.TipsResponse, error) {
	references := deps.Protocol.MainEngineInstance().TipSelection.SelectTips(int(req.GetCount()))

	return &inx.TipsResponse{
		StrongTips:      inx.NewBlockIds(references[iotago.StrongParentType]),
		WeakTips:        inx.NewBlockIds(references[iotago.WeakParentType]),
		ShallowLikeTips: inx.NewBlockIds(references[iotago.ShallowLikeParentType]),
	}, nil
}

func (s *Server) ValidatePayload(ctx context.Context, payload *inx.RawPayload) (*inx.PayloadValidationResponse, error) {
	blockPayload, err := payload.Unwrap(deps.Protocol.CurrentAPI(), serix.WithValidation())
	if err != nil {
		return &inx.PayloadValidationResponse{IsValid: false, Error: err.Error()}, nil
	}

	tx, ok := blockPayload.(*iotago.SignedTransaction)
	if !ok {
		return &inx.PayloadValidationResponse{IsValid: false, Error: err.Error()}, nil
	}

	if err := deps.Protocol.MainEngineInstance().Ledger.ValidateTransactionInVM(ctx, tx); err != nil {
		return &inx.PayloadValidationResponse{IsValid: false, Error: err.Error()}, nil
	}

	return &inx.PayloadValidationResponse{IsValid: true}, nil
}
