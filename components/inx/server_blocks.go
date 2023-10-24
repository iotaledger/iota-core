package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Server) ReadBlock(_ context.Context, blockID *inx.BlockId) (*inx.RawBlock, error) {
	blkID := blockID.Unwrap()
	block, exists := deps.Protocol.MainEngineInstance().Block(blkID) // block +1
	if !exists {
		return nil, status.Errorf(codes.NotFound, "block %s not found", blkID.ToHex())
	}

	return &inx.RawBlock{
		Data: block.Data(),
	}, nil
}

func (s *Server) ReadBlockMetadata(_ context.Context, blockID *inx.BlockId) (*inx.BlockMetadata, error) {
	return getINXBlockMetadata(blockID.Unwrap())
}

func (s *Server) ListenToBlocks(_ *inx.NoParams, srv inx.INX_ListenToBlocksServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToBlocks", workerpool.WithWorkerCount(workerCount)).Start()

	unhook := deps.Protocol.Events.Engine.Booker.BlockBooked.Hook(func(block *blocks.Block) {
		payload := inx.NewBlockWithBytes(block.ID(), block.ModelBlock().Data())
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

func (s *Server) ListenToAcceptedBlocks(_ *inx.NoParams, srv inx.INX_ListenToAcceptedBlocksServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToAcceptedBlocks", workerpool.WithWorkerCount(workerCount)).Start()

	unhook := deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
		payload, err := getINXBlockMetadata(block.ID())
		if err != nil {
			Component.LogErrorf("get block metadata error: %v", err)
			cancel()
		}

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

func (s *Server) ListenToConfirmedBlocks(_ *inx.NoParams, srv inx.INX_ListenToConfirmedBlocksServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToConfirmedBlocks", workerpool.WithWorkerCount(workerCount)).Start()

	unhook := deps.Protocol.Events.Engine.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
		payload, err := getINXBlockMetadata(block.ID())
		if err != nil {
			Component.LogErrorf("get block metadata error: %v", err)
			cancel()
		}

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

func (s *Server) SubmitBlock(ctx context.Context, rawBlock *inx.RawBlock) (*inx.BlockId, error) {
	block, err := rawBlock.UnwrapBlock(deps.Protocol)
	if err != nil {
		return nil, err
	}

	return s.attachBlock(ctx, block)
}

func (s *Server) attachBlock(ctx context.Context, block *iotago.ProtocolBlock) (*inx.BlockId, error) {
	mergedCtx, mergedCtxCancel := contextutils.MergeContexts(ctx, Component.Daemon().ContextStopped())
	defer mergedCtxCancel()

	blockID, err := deps.BlockHandler.AttachBlock(mergedCtx, block)
	if err != nil {
		switch {
		case ierrors.Is(err, blockhandler.ErrBlockAttacherInvalidBlock):
			return nil, status.Errorf(codes.InvalidArgument, "failed to attach block: %s", err.Error())

		case ierrors.Is(err, blockhandler.ErrBlockAttacherAttachingNotPossible):
			return nil, status.Errorf(codes.Internal, "failed to attach block: %s", err.Error())

		default:
			return nil, status.Errorf(codes.Internal, "failed to attach block: %s", err.Error())
		}
	}

	return inx.NewBlockId(blockID), nil
}

func getINXBlockMetadata(blockID iotago.BlockID) (*inx.BlockMetadata, error) {
	blockMetadata, err := deps.Protocol.MainEngineInstance().Retainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Errorf("failed to get BlockMetadata: %v", err)
	}

	return &inx.BlockMetadata{
		BlockId:                  inx.NewBlockId(blockID),
		BlockState:               inx.WrapBlockState(blockMetadata.BlockState),
		BlockFailureReason:       inx.WrapBlockFailureReason(blockMetadata.BlockFailureReason),
		TransactionState:         inx.WrapTransactionState(blockMetadata.TransactionState),
		TransactionFailureReason: inx.WrapTransactionFailureReason(blockMetadata.TransactionFailureReason),
	}, nil
}
