package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
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

func (s *Server) ListenToLatestCommitments(_ *inx.NoParams, srv inx.INX_ListenToLatestCommitmentsServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToLatestCommitments", workerCount).Start()

	unhook := deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(func(commitmentDetails *notarization.SlotCommittedDetails) {
		payload := inx.NewCommitmentWithBytes(commitmentDetails.Commitment.ID(), commitmentDetails.Commitment.Data())

		if err := srv.Send(payload); err != nil {
			Component.LogErrorf("send error: %v", err)
			cancel()
		}
	}, event.WithWorkerPool(wp)).Unhook

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return ctx.Err()
}

func (s *Server) ListenToFinalizedCommitments(_ *inx.NoParams, srv inx.INX_ListenToFinalizedCommitmentsServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToFinalizedCommitments", workerCount).Start()

	unhook := deps.Protocol.Events.Engine.SlotGadget.SlotFinalized.Hook(func(index iotago.SlotIndex) {
		payload, err := deps.Protocol.MainEngineInstance().Storage.Commitments().Load(index)
		if err != nil {
			Component.LogErrorf("load commitment error: %v", err)
			cancel()
		}

		inxCommitment := inx.NewCommitmentWithBytes(payload.ID(), payload.Data())

		if err := srv.Send(inxCommitment); err != nil {
			Component.LogErrorf("send error: %v", err)
			cancel()
		}
	}, event.WithWorkerPool(wp)).Unhook

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return ctx.Err()
}
