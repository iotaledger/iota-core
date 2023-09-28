package inx

import (
	"context"

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
