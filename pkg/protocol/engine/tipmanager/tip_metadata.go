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

	// TipPool exposes a Receptor that holds information about the TipPool the block is part of.
	TipPool() agential.Receptor[TipPool]

	// LivenessThresholdReached exposes a Receptor that receives a value of true if the liveness threshold is reached.
	LivenessThresholdReached() agential.Receptor[bool]

	// IsStrongTip returns true if the block is an unreferenced strong tip.
	IsStrongTip() agential.ReadOnlyReceptor[bool]

	// IsWeakTip returns true if the block is an unreferenced weak tip.
	IsWeakTip() agential.ReadOnlyReceptor[bool]

	// IsOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
	IsOrphaned() agential.ReadOnlyReceptor[bool]

	Evicted() agential.ReadOnlyReceptor[bool]
}
