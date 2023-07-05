package tipmanager

import (
	"github.com/iotaledger/iota-core/pkg/core/agential"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata allows to access the tip related metadata and events of a block in the TipManager.
type TipMetadata interface {
	// ID returns the identifier of the block the TipMetadata belongs to.
	ID() iotago.BlockID

	// Block returns the block that the TipMetadata belongs to.
	Block() *blocks.Block

	// TipPool exposes a ValueReceptor that stores the current TipPool of the block.
	TipPool() *agential.ValueReceptor[TipPool]

	// IsLivenessThresholdReached exposes a ValueReceptor that stores if the liveness threshold was reached.
	IsLivenessThresholdReached() *agential.ValueReceptor[bool]

	// IsStrongTip returns a Value that indicates if the block is a strong tip.
	IsStrongTip() agential.Value[bool]

	// IsWeakTip returns a Value that indicates if the block is a weak tip.
	IsWeakTip() agential.Value[bool]

	// IsOrphaned returns a Value that indicates if the block was orphaned.
	IsOrphaned() agential.Value[bool]

	// IsEvicted returns a Value that indicates if the block was evicted.
	IsEvicted() agential.Value[bool]
}
