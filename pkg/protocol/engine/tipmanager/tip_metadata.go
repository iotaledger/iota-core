package tipmanager

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata allows to access the tip related metadata and events of a block in the TipManager.
type TipMetadata interface {
	// ID returns the identifier of the block the TipMetadata belongs to.
	ID() iotago.BlockID

	// Block returns the block that the TipMetadata belongs to.
	Block() *blocks.Block

	// TipPool exposes a variable that stores the current TipPool of the block.
	TipPool() reactive.Variable[TipPool]

	// IsStrongTip returns a ReadableVariable that indicates if the block is a strong tip.
	IsStrongTip() reactive.ReadableVariable[bool]

	// IsWeakTip returns a ReadableVariable that indicates if the block is a weak tip.
	IsWeakTip() reactive.ReadableVariable[bool]

	// IsOrphaned returns a ReadableVariable that indicates if the block was orphaned.
	IsOrphaned() reactive.ReadableVariable[bool]

	// LivenessThresholdReached exposes an event that is triggered when the liveness threshold is reached.
	LivenessThresholdReached() reactive.Event

	// Evicted exposes an event that is triggered when the block is evicted.
	Evicted() reactive.Event

	String() string
}
