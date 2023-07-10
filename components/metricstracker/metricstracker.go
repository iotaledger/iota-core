package metricstracker

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/metrics"
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
	bpsLock           syncutils.RWMutex
	cbps              float64
	cbpsLock          syncutils.RWMutex
	confirmedRate     float64
	confirmedRateLock syncutils.RWMutex

	isBootstrappedFunc isBootstrappedFunc
}

func New(bootstrappedFunc isBootstrappedFunc) *MetricsTracker {
	return &MetricsTracker{
		isBootstrappedFunc: bootstrappedFunc,
		metrics:            &metrics.ServerMetrics{},
		oldTime:            time.Now().Unix(),
	}
}

func (m *MetricsTracker) NodeMetrics() *NodeMetrics {
	if !m.isBootstrappedFunc() {
		return &NodeMetrics{}
	}

	m.bpsLock.RLock()
	defer m.bpsLock.RUnlock()
	m.cbpsLock.RLock()
	defer m.cbpsLock.RUnlock()
	m.confirmedRateLock.RLock()
	defer m.confirmedRateLock.RUnlock()

	return &NodeMetrics{
		BlocksPerSecond:          m.bps,
		ConfirmedBlocksPerSecond: m.cbps,
		ConfirmedRate:            m.confirmedRate,
	}
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

type NodeMetrics struct {
	BlocksPerSecond          float64
	ConfirmedBlocksPerSecond float64
	ConfirmedRate            float64
}
