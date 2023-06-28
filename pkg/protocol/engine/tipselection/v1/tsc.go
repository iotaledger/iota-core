package tipselectionv1

import (
	"fmt"
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
		for _, tip := range t.tscQueue.PopUntil(newThreshold) {
			// TODO: SET TSCThresholdReached()
			fmt.Println(tip)
		}
	})

	return t
}

func (t *TSCManager) Add(tip tipmanager.TipMetadata) {
	t.tscThreshold.Read(func(tscThreshold time.Time) {
		if tscThreshold.After(tip.Block().IssuingTime()) {
			// TODO: SET TSCThresholdReached()
		} else {
			t.tscQueue.Push(tip, tip.Block().IssuingTime())
		}
	})
}

func (t *TSCManager) SetTSCThreshold(newThreshold time.Time) {
	t.tscThreshold.Compute(func(currentThreshold time.Time) time.Time {
		if newThreshold.Before(currentThreshold) {
			return currentThreshold
		}

		return newThreshold
	})
}
