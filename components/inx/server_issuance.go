package inx

import (
	"context"

	"github.com/iotaledger/hive.go/serializer/v2/serix"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
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
	blockPayload, unwrapErr := payload.Unwrap(deps.Protocol.CurrentAPI(), serix.WithValidation())
	if unwrapErr != nil {
		//nolint:nilerr // this is expected behavior
		return &inx.PayloadValidationResponse{IsValid: false, Error: unwrapErr.Error()}, nil
	}

	switch payload := blockPayload.(type) {
	case *iotago.SignedTransaction:
		memPool := deps.Protocol.MainEngineInstance().Ledger.MemPool()
		inputReferences, inputsErr := memPool.VM().Inputs(payload.Transaction)
		if inputsErr != nil {
			//nolint:nilerr // this is expected behavior
			return &inx.PayloadValidationResponse{IsValid: false, Error: inputsErr.Error()}, nil
		}

		loadedInputs := make([]mempool.State, 0)

		for _, inputReference := range inputReferences {
			metadata, metadataErr := memPool.StateMetadata(inputReference)
			if metadataErr != nil {
				//nolint:nilerr // this is expected behavior
				return &inx.PayloadValidationResponse{IsValid: false, Error: metadataErr.Error()}, nil
			}

			loadedInputs = append(loadedInputs, metadata.State())
		}

		executionContext, validationErr := memPool.VM().ValidateSignatures(payload, loadedInputs)
		if validationErr != nil {
			//nolint:nilerr // this is expected behavior
			return &inx.PayloadValidationResponse{IsValid: false, Error: validationErr.Error()}, nil
		}

		_, executionErr := memPool.VM().Execute(executionContext, payload.Transaction)
		if executionErr != nil {
			//nolint:nilerr // this is expected behavior
			return &inx.PayloadValidationResponse{IsValid: false, Error: executionErr.Error()}, nil
		}

		return &inx.PayloadValidationResponse{IsValid: true}, nil

	case *iotago.TaggedData:
		// TaggedData is always valid if serix decoding was successful
		return &inx.PayloadValidationResponse{IsValid: true}, nil

	default:
		return &inx.PayloadValidationResponse{IsValid: false, Error: "given payload type unknown"}, nil
	}
}
