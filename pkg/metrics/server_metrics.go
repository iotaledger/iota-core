package metrics

import "go.uber.org/atomic"

// ServerMetrics defines metrics over the entire runtime of the node.
type ServerMetrics struct {
	// The number of blocks that have passed the filters.
	Blocks atomic.Uint64
	// The number of confirmed blocks.
	ConfirmedBlocks atomic.Uint64
}
