package tipmanager

import (
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipMetadata allows to access the tip related metadata and events of a block in the TipManager.
type TipMetadata interface {
	// ID returns the ID of the Block the TipMetadata belongs to.
	ID() iotago.BlockID

	// Block returns the Block that the TipMetadata belongs to.
	Block() *blocks.Block

	// TipPool returns the TipPool the block is currently in.
	TipPool() TipPool

	// OnTipPoolUpdated registers a callback that is triggered when the TipPool of the block is updated.
	OnTipPoolUpdated(handler func(tipPool TipPool)) (unsubscribe func())

	// IsStrongTip returns true if the block is part of the strong tip set.
	IsStrongTip() bool

	// OnIsStrongTipUpdated registers a callback that is triggered when the IsStrongTip property of the block is updated.
	OnIsStrongTipUpdated(handler func(isStrongTip bool)) (unsubscribe func())

	// IsWeakTip returns true if the block is part of the weak tip set.
	IsWeakTip() bool

	// OnIsWeakTipUpdated registers a callback that is triggered when the IsWeakTip property of the block is updated.
	OnIsWeakTipUpdated(handler func(isWeakTip bool)) (unsubscribe func())

	// IsEvicted returns true if the block was evicted from the TipManager.
	IsEvicted() bool

	// OnEvicted registers a callback that is triggered when the block is evicted from the TipManager.
	OnEvicted(handler func())
}
