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

	// TipPool returns the current TipPool of the block.
	TipPool() agential.Receptor[TipPool]

	// SetLivenessThresholdReached marks the block as having reached the liveness threshold.
	SetLivenessThresholdReached()

	// OnLivenessThresholdReached registers a callback that is triggered when the block reaches the liveness threshold.
	OnLivenessThresholdReached(handler func()) (unsubscribe func())

	// IsLivenessThresholdReached returns true if the block reached the liveness threshold.
	IsLivenessThresholdReached() bool

	// IsStrongTip returns true if the block is an unreferenced strong tip.
	IsStrongTip() bool

	// OnIsStrongTipUpdated registers a callback that is triggered when the IsStrongTip property changes.
	OnIsStrongTipUpdated(handler func(isStrongTip bool)) (unsubscribe func())

	// IsWeakTip returns true if the block is an unreferenced weak tip.
	IsWeakTip() bool

	// OnIsWeakTipUpdated registers a callback that is triggered when the IsWeakTip property changes.
	OnIsWeakTipUpdated(handler func(isWeakTip bool)) (unsubscribe func())

	// IsOrphaned returns true if the block is marked orphaned or if it has an orphaned strong parent.
	IsOrphaned() bool

	// OnIsOrphanedUpdated registers a callback that is triggered when the IsOrphaned property changes.
	OnIsOrphanedUpdated(handler func(orphaned bool)) (unsubscribe func())

	Evicted() agential.ReadOnlyReceptor[bool]
}
