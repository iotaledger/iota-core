package inx

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/contextutils"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	inx "github.com/iotaledger/inx/go"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

func (s *Server) ReadActiveRootBlocks(_ context.Context, _ *inx.NoParams) (*inx.RootBlocksResponse, error) {
	latestRootBlockID, latestRootBlockSlotCommitmentID := deps.Protocol.Engines.Main.Get().EvictionState.LatestActiveRootBlock()

	// TODO: we should maybe change the INX API to return only the latest rootblock instead of the whole active set.
	return inx.WrapRootBlocks(map[iotago.BlockID]iotago.CommitmentID{
		latestRootBlockID: latestRootBlockSlotCommitmentID,
	}), nil
}

func (s *Server) ReadBlock(_ context.Context, blockID *inx.BlockId) (*inx.RawBlock, error) {
	blkID := blockID.Unwrap()
	block, exists := deps.Protocol.Engines.Main.Get().Block(blkID) // block +1
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

	unhook := deps.Protocol.Events.Engine.BlockRetainer.BlockRetained.Hook(func(block *blocks.Block) {
		payload := inx.NewBlockWithBytes(block.ID(), block.ModelBlock().Data())

		if ctx.Err() != nil {
			// context is done, so we don't need to send the payload
			return
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

func (s *Server) ListenToBlockMetadata(_ *inx.NoParams, srv inx.INX_ListenToBlockMetadataServer) error {
	ctx, cancel := context.WithCancel(Component.Daemon().ContextStopped())

	wp := workerpool.New("ListenToBlockMetadata", workerpool.WithWorkerCount(workerCount)).Start()

	sendBlockMetadata := func(blockMetadata *api.BlockMetadataResponse) {
		payload := inx.WrapBlockMetadata(blockMetadata)

		if ctx.Err() != nil {
			// context is done, so we don't need to send the payload
			return
		}

		if err := srv.Send(payload); err != nil {
			Component.LogErrorf("send error: %v", err)
			cancel()
		}
	}

	unhook := lo.Batch(
		deps.Protocol.Events.Engine.BlockRetainer.BlockRetained.Hook(func(block *blocks.Block) {
			sendBlockMetadata(&api.BlockMetadataResponse{
				BlockID:    block.ID(),
				BlockState: api.BlockStatePending,
			})
		}, event.WithWorkerPool(wp)).Unhook,
		deps.Protocol.Events.Engine.BlockRetainer.BlockAccepted.Hook(func(block *blocks.Block) {
			sendBlockMetadata(&api.BlockMetadataResponse{
				BlockID:    block.ID(),
				BlockState: api.BlockStateAccepted,
			})
		}, event.WithWorkerPool(wp)).Unhook,
		deps.Protocol.Events.Engine.BlockRetainer.BlockConfirmed.Hook(func(block *blocks.Block) {
			sendBlockMetadata(&api.BlockMetadataResponse{
				BlockID:    block.ID(),
				BlockState: api.BlockStateConfirmed,
			})
		}, event.WithWorkerPool(wp)).Unhook,
		deps.Protocol.Events.Engine.BlockRetainer.BlockDropped.Hook(func(block *blocks.Block) {
			sendBlockMetadata(&api.BlockMetadataResponse{
				BlockID:    block.ID(),
				BlockState: api.BlockStateDropped,
			})
		}, event.WithWorkerPool(wp)).Unhook,
	)

	<-ctx.Done()
	unhook()

	// We need to wait until all tasks are done, otherwise we might call
	// "SendMsg" and "CloseSend" in parallel on the grpc stream, which is
	// not safe according to the grpc docs.
	wp.Shutdown()
	wp.ShutdownComplete.Wait()

	return ctx.Err()
}

func (s *Server) ReadAcceptedBlocks(slot *inx.SlotRequest, srv inx.INX_ReadAcceptedBlocksServer) error {
	blocksStore, err := deps.Protocol.Engines.Main.Get().Storage.Blocks(slot.Unwrap())
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to get blocks: %s", err.Error())
	}

	if err := blocksStore.ForEachBlockInSlot(func(block *model.Block) error {
		metadata, err := getINXBlockMetadata(block.ID())
		if err != nil {
			return err
		}

		payload := &inx.BlockWithMetadata{
			Metadata: metadata,
			Block: &inx.RawBlock{
				Data: block.Data(),
			},
		}

		return srv.Send(payload)
	}); err != nil {
		return status.Errorf(codes.Internal, "failed to iterate blocks: %s", err.Error())
	}

	return nil
}

func (s *Server) SubmitBlock(ctx context.Context, rawBlock *inx.RawBlock) (*inx.BlockId, error) {
	block, err := rawBlock.UnwrapBlock(deps.Protocol)
	if err != nil {
		return nil, err
	}

	return s.attachBlock(ctx, block)
}

func (s *Server) attachBlock(ctx context.Context, block *iotago.Block) (*inx.BlockId, error) {
	mergedCtx, mergedCtxCancel := contextutils.MergeContexts(ctx, Component.Daemon().ContextStopped())
	defer mergedCtxCancel()

	blockID, err := deps.RequestHandler.SubmitBlockAndAwaitRetainer(mergedCtx, block)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to attach block: %s", err.Error())
	}

	return inx.NewBlockId(blockID), nil
}

func getINXBlockMetadata(blockID iotago.BlockID) (*inx.BlockMetadata, error) {
	blockMetadata, err := deps.Protocol.Engines.Main.Get().BlockRetainer.BlockMetadata(blockID)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get BlockMetadata")
	}

	return inx.WrapBlockMetadata(blockMetadata), nil
}
