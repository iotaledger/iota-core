package tipselectionv1

import (
	"time"

	"github.com/iotaledger/hive.go/lo"
	lpromise "github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/timed"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
)

type TSCManager struct {
	queue                     timed.PriorityQueue[tipmanager.TipMetadata]
	blockIssuingTimeThreshold *lpromise.Value[time.Time]
}

func NewTSCManager() *TSCManager {
	t := &TSCManager{
		queue:                     timed.NewPriorityQueue[tipmanager.TipMetadata](true),
		blockIssuingTimeThreshold: lpromise.NewValue[time.Time](),
	}

	t.blockIssuingTimeThreshold.OnUpdate(func(_, newThreshold time.Time) {
		t.markTipsBlockIssuingTimeThresholdReached(newThreshold)
	})

	return t
}

func (t *TSCManager) Add(tip tipmanager.TipMetadata) {
	t.blockIssuingTimeThreshold.Read(func(threshold time.Time) {
		if threshold.After(tip.Block().IssuingTime()) {
			// TODO: tip.SetBlockIssuingTimeThresholdReached()
		} else {
			t.queue.Push(tip, tip.Block().IssuingTime())
		}
	})
}

func (t *TSCManager) UpdateBlockIssuingTimeThreshold(newThreshold time.Time) {
	t.blockIssuingTimeThreshold.Compute(func(currentThreshold time.Time) time.Time {
		return lo.Cond(newThreshold.Before(currentThreshold), currentThreshold, newThreshold)
	})
}

func (t *TSCManager) markTipsBlockIssuingTimeThresholdReached(threshold time.Time) {
	for _, _ = range t.queue.PopUntil(threshold) {
		// TODO: tip.SetBlockIssuingTimeThresholdReached()
	}
}
