package inx

import (
	"context"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
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

func (s *Server) ValidatePayload(_ context.Context, payload *inx.RawPayload) (*inx.PayloadValidationResponse, error) {
	if err := func() error {
		blockPayload, unwrapErr := payload.Unwrap(deps.Protocol.CurrentAPI(), serix.WithValidation())
		if unwrapErr != nil {
			return unwrapErr
		}

		switch typedPayload := blockPayload.(type) {
		case *iotago.SignedTransaction:
			memPool := deps.Protocol.MainEngineInstance().Ledger.MemPool()

			inputReferences, inputsErr := memPool.VM().Inputs(typedPayload.Transaction)
			if inputsErr != nil {
				return inputsErr
			}

			loadedInputs := make([]mempool.State, 0)
			for _, inputReference := range inputReferences {
				if metadata, metadataErr := memPool.StateMetadata(inputReference); metadataErr != nil {
					return metadataErr
				} else {
					loadedInputs = append(loadedInputs, metadata.State())
				}
			}

			if executionContext, validationErr := memPool.VM().ValidateSignatures(typedPayload, loadedInputs); validationErr != nil {
				return validationErr
			} else {
				return lo.Return2(memPool.VM().Execute(executionContext, typedPayload.Transaction))
			}

		case *iotago.TaggedData:
			// TaggedData is always valid if serix decoding was successful
			return nil

		default:
			return ierrors.Errorf("unsupported payload type: %T", typedPayload)
		}
	}(); err != nil {
		//nolint:nilerr // this is expected behavior
		return &inx.PayloadValidationResponse{IsValid: false, Error: err.Error()}, nil
	}

	return &inx.PayloadValidationResponse{IsValid: true}, nil
}
