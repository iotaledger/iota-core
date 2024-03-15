package inx

import (
	"context"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) RequestTips(_ context.Context, req *inx.TipsRequest) (*inx.TipsResponse, error) {
	references := deps.Protocol.Engines.Main.Get().TipSelection.SelectTips(int(req.GetCount()))

	return &inx.TipsResponse{
		StrongTips:      inx.NewBlockIds(references[iotago.StrongParentType]),
		WeakTips:        inx.NewBlockIds(references[iotago.WeakParentType]),
		ShallowLikeTips: inx.NewBlockIds(references[iotago.ShallowLikeParentType]),
	}, nil
}

func (s *Server) ValidatePayload(_ context.Context, payload *inx.RawPayload) (*inx.PayloadValidationResponse, error) {
	if err := func() error {
		blockPayload, unwrapErr := payload.Unwrap(deps.Protocol.CommittedAPI(), serix.WithValidation())
		if unwrapErr != nil {
			return unwrapErr
		}

		switch typedPayload := blockPayload.(type) {
		case *iotago.SignedTransaction:
			memPool := deps.Protocol.Engines.Main.Get().Ledger.MemPool()

			inputReferences, inputsErr := memPool.VM().Inputs(typedPayload.Transaction)
			if inputsErr != nil {
				return inputsErr
			}

			loadedInputs := make([]mempool.State, 0)
			for _, inputReference := range inputReferences {
				metadata, metadataErr := memPool.StateMetadata(inputReference)
				if metadataErr != nil {
					return metadataErr
				}

				loadedInputs = append(loadedInputs, metadata.State())
			}

			executionContext, validationErr := memPool.VM().ValidateSignatures(typedPayload, loadedInputs)
			if validationErr != nil {
				return validationErr
			}

			return lo.Return2(memPool.VM().Execute(executionContext, typedPayload.Transaction))

		case *iotago.TaggedData:
			// TaggedData is always valid if serix decoding was successful
			return nil

		case *iotago.CandidacyAnnouncement:
			panic("TODO: implement me")
		default:
			// We're switching on the Go payload type here, so we can only run into the default case
			// if we added a new payload type and have not handled it above. In this case we want to panic.
			panic("all supported payload types should be handled above")
		}
	}(); err != nil {
		//nolint:nilerr // this is expected behavior
		return &inx.PayloadValidationResponse{IsValid: false, Error: err.Error()}, nil
	}

	return &inx.PayloadValidationResponse{IsValid: true}, nil
}
