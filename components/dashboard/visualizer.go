package dashboard

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
)

// var (
// 	currentSlot atomic.Int64
// )

// vertex defines a vertex in a DAG.
type vertex struct {
	ID                  string   `json:"id"`
	StrongParents       []string `json:"strongParents"`
	WeakParents         []string `json:"weakParents"`
	ShallowLikedParents []string `json:"shallowLikedParents"`
	IsFinalized         bool     `json:"is_finalized"`
	IsTx                bool     `json:"is_tx"`
	IssuingTime         time.Time
}

// tipinfo holds information about whether a given block is a tip or not.
type tipinfo struct {
	ID    string `json:"id"`
	IsTip bool   `json:"is_tip"`
}

// history holds a set of vertices in a DAG.
// type history struct {
// 	Vertices []vertex `json:"vertices"`
// }

func sendVertex(blk *blocks.Block, finalized bool) {
	broadcastWsBlock(&wsblk{MsgTypeVertex, &vertex{
		ID:            blk.ID().ToHex(),
		StrongParents: blk.Block().StrongParents.ToHex(),
		WeakParents:   blk.Block().WeakParents.ToHex(),
		IsFinalized:   finalized,
		// IsTx:          blk.Payload().Type() == devnetvm.TransactionType,
	}}, true)
}

func sendTipInfo(block *blocks.Block, isTip bool) {
	broadcastWsBlock(&wsblk{MsgTypeTipInfo, &tipinfo{
		ID:    block.ID().ToHex(),
		IsTip: isTip,
	}}, true)
}

func runVisualizer(component *app.Component) {
	if err := component.Daemon().BackgroundWorker("Dashboard[Visualizer]", func(ctx context.Context) {
		unhook := lo.Batch(
			deps.Protocol.Events.Engine.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
				sendVertex(block, false)
				// if block.ID().Index() > slot.Index(currentSlot.Load()) {
				// 	currentSlot.Store(int64(block.ID().Index()))
				// }
			}, event.WithWorkerPool(component.WorkerPool)).Unhook,
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				sendVertex(block, block.IsAccepted())
			}, event.WithWorkerPool(component.WorkerPool)).Unhook,
			deps.Protocol.Events.Engine.TipManager.BlockAdded.Hook(func(tipMetadata tipmanager.TipMetadata) {
				sendTipInfo(tipMetadata.Block(), true)

				tipMetadata.Evicted().OnUpdate(func(_, _ bool) {
					sendTipInfo(tipMetadata.Block(), false)
				})
			}, event.WithWorkerPool(component.WorkerPool)).Unhook,
		)
		<-ctx.Done()
		component.LogInfo("Stopping Dashboard[Visualizer] ...")
		unhook()
		component.LogInfo("Stopping Dashboard[Visualizer] ... done")
	}, daemon.PriorityDashboard); err != nil {
		component.LogPanicf("Failed to start as daemon: %s", err)
	}
}

// func setupVisualizerRoutes(routeGroup *echo.Group) {
// 	routeGroup.GET("/visualizer/history", func(c echo.Context) (err error) {
// 		var res []vertex

// 		start := slot.Index(currentSlot.Load())
// 		for _, ei := range []slot.Index{start - 1, start} {
// 			blocks := deps.Retainer.LoadAllBlockMetadata(ei)
// 			_ = blocks.ForEach(func(element *retainer.BlockMetadata) (err error) {
// 				res = append(res, vertex{
// 					ID:              element.ID().Base58(),
// 					ParentIDsByType: prepareParentReferences(element.M.Block),
// 					IsFinalized:     element.M.PreAccepted,
// 					IsTx:            element.M.Block.Payload().Type() == devnetvm.TransactionType,
// 					IssuingTime:     element.M.Block.IssuingTime(),
// 				})
// 				return
// 			})
// 		}

// 		sort.Slice(res, func(i, j int) bool {
// 			return res[i].IssuingTime.Before(res[j].IssuingTime)
// 		})

// 		return c.JSON(http.StatusOK, history{Vertices: res})
// 	})
// }
