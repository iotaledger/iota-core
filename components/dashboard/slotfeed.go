package dashboard

import (
	"context"

	"github.com/iotaledger/hive.go/app"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/daemon"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SlotInfo struct {
	Index iotago.SlotIndex `json:"index"`
	ID    string           `json:"id"`
}

func runSlotsLiveFeed(component *app.Component) {
	if err := component.Daemon().BackgroundWorker("Dashboard[SlotsLiveFeed]", func(ctx context.Context) {
		hook := deps.Protocol.Events.Engine.Notarization.SlotCommitted.Hook(onSlotCommitted, event.WithWorkerPool(component.WorkerPool))

		<-ctx.Done()

		component.LogInfo("Stopping Dashboard[SlotsLiveFeed] ...")
		hook.Unhook()
		component.LogInfo("Stopping Dashboard[SlotsLiveFeed] ... done")
	}, daemon.PriorityDashboard); err != nil {
		component.LogPanicf("Failed to start as daemon: %s", err)
	}
}

func onSlotCommitted(details *notarization.SlotCommittedDetails) {
	broadcastWsBlock(&wsblk{MsgTypeSlotInfo, &SlotInfo{Index: details.Commitment.Index(), ID: details.Commitment.ID().ToHex()}})
}
