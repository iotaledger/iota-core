package trivialmetricstracker

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/metrics"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	"github.com/iotaledger/iota-core/pkg/protocol/metricstracker"
)

type (
	isBootstrappedFunc func() bool
)

type MetricsTracker struct {
	metrics *metrics.ServerMetrics

	oldBlockCount          uint64
	oldConfirmedBlockCount uint64
	oldTime                int64

	bps               float64
	bpsLock           sync.RWMutex
	cbps              float64
	cbpsLock          sync.RWMutex
	confirmedRate     float64
	confirmedRateLock sync.RWMutex

	isBootstrappedFunc isBootstrappedFunc

	module.Module
}

// NewProvider creates a new MetricsTracker provider.
func NewProvider(opts ...options.Option[MetricsTracker]) module.Provider[*engine.Engine, metricstracker.MetricsTracker] {
	return module.Provide(func(e *engine.Engine) metricstracker.MetricsTracker {
		m := New(e.IsBootstrapped)
		asyncOpt := event.WithWorkerPool(e.Workers.CreatePool("MetricsTracker", 1))

		e.Events.BlockDAG.BlockAttached.Hook(func(block *blocks.Block) {
			m.metrics.Blocks.Inc()
		}, asyncOpt)

		e.Events.Notarization.SlotCommitted.Hook(func(_ *notarization.SlotCommittedDetails) {
			m.measure()
		})

		e.Events.BlockGadget.BlockConfirmed.Hook(func(b *blocks.Block) {
			m.metrics.ConfirmedBlocks.Inc()
		}, asyncOpt)

		m.TriggerInitialized()

		return m
	})
}

func New(bootstrappedFunc isBootstrappedFunc) *MetricsTracker {
	return &MetricsTracker{
		isBootstrappedFunc: bootstrappedFunc,
		metrics:            &metrics.ServerMetrics{},
		oldTime:            time.Now().Unix(),
	}
}

func (m *MetricsTracker) NodeMetrics() *metricstracker.NodeMetrics {
	if !m.isBootstrappedFunc() {
		return &metricstracker.NodeMetrics{}
	}

	m.bpsLock.RLock()
	defer m.bpsLock.RUnlock()
	m.cbpsLock.RLock()
	defer m.cbpsLock.RUnlock()
	m.confirmedRateLock.RLock()
	defer m.confirmedRateLock.RUnlock()

	return &metricstracker.NodeMetrics{
		BlocksPerSecond:          m.bps,
		ConfirmedBlocksPerSecond: m.cbps,
		ConfirmedRate:            m.confirmedRate,
	}
}

func (m *MetricsTracker) Shutdown() {
	m.TriggerStopped()
}

// measures the received BPS, CBPS and confirmationRate value.
func (m *MetricsTracker) measure() {
	newTime := time.Now().Unix()
	timeDiff := newTime - m.oldTime
	if timeDiff <= 0 {
		return
	}
	m.oldTime = newTime

	m.bpsLock.Lock()
	defer m.bpsLock.Unlock()
	m.cbpsLock.Lock()
	defer m.cbpsLock.Unlock()
	m.confirmedRateLock.Lock()
	defer m.confirmedRateLock.Unlock()

	// calculate BPS.
	newBlockCount := m.metrics.Blocks.Load()
	blocksDiff := newBlockCount - m.oldBlockCount
	m.oldBlockCount = newBlockCount
	m.bps = float64(blocksDiff) / float64(timeDiff)

	// calculate confirmed BPS.
	newConfirmedBlockCount := m.metrics.ConfirmedBlocks.Load()
	confirmedBlocksDiff := newConfirmedBlockCount - m.oldConfirmedBlockCount
	m.oldConfirmedBlockCount = newConfirmedBlockCount
	m.cbps = float64(confirmedBlocksDiff) / float64(timeDiff)

	// calculate confirmed rate.
	m.confirmedRate = (float64(confirmedBlocksDiff) / float64(blocksDiff)) * 100.0
}
