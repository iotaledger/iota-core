package tipselectionv1

import (
	"time"

	lpromise "github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/timed"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
)

type TSCManager struct {
	tscQueue     timed.PriorityQueue[tipmanager.TipMetadata]
	tscThreshold *lpromise.Value[time.Time]
}

func NewTSCManager() *TSCManager {
	t := &TSCManager{
		tscQueue:     timed.NewPriorityQueue[tipmanager.TipMetadata](true),
		tscThreshold: lpromise.NewValue[time.Time](),
	}

	t.tscThreshold.OnUpdate(func(_, newThreshold time.Time) {
		t.markTipsPastThreshold(newThreshold)
	})

	return t
}

func (t *TSCManager) Add(tip tipmanager.TipMetadata) {
	t.tscThreshold.Read(func(threshold time.Time) {
		if threshold.After(tip.Block().IssuingTime()) {
			// TODO: tip.SetTSCThresholdReached()
		} else {
			t.tscQueue.Push(tip, tip.Block().IssuingTime())
		}
	})
}

func (t *TSCManager) UpdateThreshold(newThreshold time.Time) {
	t.tscThreshold.Compute(func(currentThreshold time.Time) time.Time {
		if newThreshold.Before(currentThreshold) {
			return currentThreshold
		}

		return newThreshold
	})
}

func (t *TSCManager) markTipsPastThreshold(threshold time.Time) {
	for _, _ = range t.tscQueue.PopUntil(threshold) {
		// TODO: tip.SetTSCThresholdReached()
	}
}
