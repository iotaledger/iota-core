package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

func inxCommitment(commitment *model.Commitment) *inx.Commitment {
	return &inx.Commitment{
		CommitmentId: inx.NewCommitmentId(commitment.ID()),
		Commitment: &inx.RawCommitment{
			Data: commitment.Data(),
		},
	}
}

func (s *Server) ReadCommitment(_ context.Context, req *inx.CommitmentRequest) (*inx.Commitment, error) {
	commitmentIndex := iotago.SlotIndex(req.GetCommitmentIndex())

	if req.GetCommitmentId() != nil {
		commitmentIndex = req.GetCommitmentId().Unwrap().Index()
	}

	commitment, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(commitmentIndex)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, status.Errorf(codes.NotFound, "commitment index %d not found", req.GetCommitmentIndex())
		}

		return nil, err
	}

	if req.GetCommitmentId() != nil {
		// If it was requested by id, make sure the id matches the commitment.
		if commitment.ID() != req.GetCommitmentId().Unwrap() {
			return nil, status.Errorf(codes.NotFound, "commitment id %s not found, found %s instead", req.GetCommitmentId().Unwrap(), commitment.ID())
		}
	}

	return inxCommitment(commitment), nil
}
