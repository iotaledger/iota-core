package metrics

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	accountNamespace = "account"

	credits     = "credits"
	activeSeats = "active_seats"
)

var AccountMetrics = collector.NewCollection(accountNamespace,
	collector.WithMetric(collector.NewMetric(credits,
		collector.WithType(collector.GaugeVec),
		collector.WithHelp("Credits per Account."),
		collector.WithLabels("account"),
		collector.WithLabelValuesCollection(),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				accountData, exists, _ := deps.Protocol.MainEngineInstance().Ledger.Account(block.ProtocolBlock().IssuerID, deps.Protocol.SyncManager.LatestCommittedSlot())
				if exists {
					deps.Collector.Update(accountNamespace, credits, collector.MultiLabels(accountData.ID.String()), float64(accountData.Credits.Value))
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(activeSeats,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Seats seen as active by the node."),
		collector.WithCollectFunc(func() map[string]float64 {
				return collector.SingleValue(deps.Protocol.MainEngineInstance().SybilProtection.SeatManager().OnlineCommittee().Size())
		}),
	)),
)
