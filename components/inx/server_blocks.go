package inx

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/blockfactory"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
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
	blkID := blockID.Unwrap()

	finalizedSlot := deps.Protocol.SyncManager.LatestFinalizedSlot()

	// Check if the block is still in the cache and we have the proper flags
	if cachedBlock, exists := deps.Protocol.MainEngineInstance().BlockFromCache(blkID); exists {
		return &inx.BlockMetadata{
			BlockId:   blockID,
			Accepted:  cachedBlock.IsAccepted(),
			Confirmed: cachedBlock.IsConfirmed(),
			Finalized: cachedBlock.ID().Index() <= finalizedSlot,
		}, nil
	}

	block, exists := deps.Protocol.MainEngineInstance().Block(blkID) // block +1
	if !exists {
		return nil, status.Errorf(codes.NotFound, "block %s not found", blkID.ToHex())
	}

	return &inx.BlockMetadata{
		BlockId:   blockID,
		Accepted:  true,  // we only store accepted blocks
		Confirmed: false, // TODO: ask the retainer for this information
		Finalized: block.ID().Index() <= finalizedSlot,
	}, nil
}

func (s *Server) ListenToBlocks(_ *inx.NoParams, srv inx.INX_ListenToBlocksServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToBlocks", workerCount).Start()

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

	wp := workerpool.New("ListenToAcceptedBlocks", workerCount).Start()

	unhook := deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
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

func (s *Server) ListenToConfirmedBlocks(_ *inx.NoParams, srv inx.INX_ListenToConfirmedBlocksServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToConfirmedBlocks", workerCount).Start()

	unhook := deps.Protocol.Events.Engine.BlockGadget.BlockConfirmed.Hook(func(block *blocks.Block) {
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

func (s *Server) SubmitBlock(ctx context.Context, rawBlock *inx.RawBlock) (*inx.BlockId, error) {
	block, err := rawBlock.UnwrapBlock(deps.Protocol.LatestAPI(), serix.WithValidation())
	if err != nil {
		return nil, err
	}

	mergedCtx, mergedCtxCancel := contextutils.MergeContexts(ctx, Component.Daemon().ContextStopped())
	defer mergedCtxCancel()

	blockID, err := deps.BlockIssuer.AttachBlock(mergedCtx, block)
	if err != nil {
		switch {
		case errors.Is(err, blockfactory.ErrBlockAttacherInvalidBlock):
			return nil, status.Errorf(codes.InvalidArgument, "failed to attach block: %s", err.Error())

		case errors.Is(err, blockfactory.ErrBlockAttacherAttachingNotPossible):
			return nil, status.Errorf(codes.Internal, "failed to attach block: %s", err.Error())

		default:
			return nil, status.Errorf(codes.Internal, "failed to attach block: %s", err.Error())
		}
	}

	return inx.NewBlockId(blockID), nil
}
