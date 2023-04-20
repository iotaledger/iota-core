package dashboardmetrics

import (
	"sync"
	"time"

	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// LatestBlockTracker is a component that tracks the ID of the latest Block.
type LatestBlockTracker struct {
	blockID iotago.BlockID
	time    time.Time
	mutex   sync.RWMutex
}

// NewLatestBlockTracker return a new LatestBlockTracker.
func NewLatestBlockTracker() (newLatestBlockTracker *LatestBlockTracker) {
	return new(LatestBlockTracker)
}

// Update updates the latest seen Block.
func (t *LatestBlockTracker) Update(block *blocks.Block) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.time.After(block.IssuingTime()) {
		return
	}

	t.blockID = block.ID()
	t.time = block.IssuingTime()
}

// BlockID returns the ID of the latest seen Block.
func (t *LatestBlockTracker) BlockID() (blockID iotago.BlockID) {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.blockID
}
