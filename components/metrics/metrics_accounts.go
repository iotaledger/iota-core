package metrics

import (
	"fmt"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/components/metrics/collector"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
)

const (
	accountNamespace = "account"

	credits = "credits"
)

var AccountMetrics = collector.NewCollection(accountNamespace,
	collector.WithMetric(collector.NewMetric(credits,
		collector.WithType(collector.GaugeVec),
		collector.WithHelp("Time since transaction issuance to the conflict acceptance"),
		collector.WithLabels("account"),
		collector.WithLabelValuesCollection(),
		collector.WithInitFunc(func() {
			deps.Protocol.Events.Engine.BlockGadget.BlockAccepted.Hook(func(block *blocks.Block) {
				accountData, exists, _ := deps.Protocol.MainEngineInstance().Ledger.Account(block.ProtocolBlock().IssuerID, deps.Protocol.SyncManager.LatestCommittedSlot())
				if exists {
					fmt.Println(">>", accountData.ID.String(), accountData.Credits.Value)
					deps.Collector.Update(accountNamespace, credits, collector.MultiLabels(accountData.ID.String()), float64(accountData.Credits.Value))
				}
			}, event.WithWorkerPool(Component.WorkerPool))
		}),
	)),
)
