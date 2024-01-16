package prometheus

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/prometheus/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	accountNamespace = "account"

	credits     = "credits"
	activeSeats = "active_seats"
)

var AccountMetrics = collector.NewCollection(accountNamespace,
	collector.WithMetric(collector.NewMetric(credits,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Credits per Account."),
		collector.WithLabels("account"),
		collector.WithPruningDelay(10*time.Minute),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				accountData, exists, _ := deps.Protocol.Engines.Main.Get().Ledger.Account(block.ProtocolBlock().Header.IssuerID, deps.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Slot())
				if exists {
					deps.Collector.Update(accountNamespace, credits, float64(accountData.Credits.Value), accountData.ID.String())
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
	collector.WithMetric(collector.NewMetric(activeSeats,
		collector.WithType(collector.Gauge),
		collector.WithHelp("Seats seen as active by the node."),
		collector.WithCollectFunc(func() (metricValue float64, labelValues []string) {
			return float64(deps.Protocol.Engines.Main.Get().SybilProtection.SeatManager().OnlineCommittee().Size()), nil
		}),
	)),
)
