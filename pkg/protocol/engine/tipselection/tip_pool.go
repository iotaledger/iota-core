package tipselection

// TipPool represents the pool a block is in.
type TipPool uint8

const (
	// UndefinedTipPool is the zero value of TipPool.
	UndefinedTipPool TipPool = iota

	// StrongTipPool represents a pool of blockMetadataStorage that are supposed to be picked up using strong references.
	StrongTipPool

	// WeakTipPool represents a pool of blockMetadataStorage that are supposed to be picked up using weak references.
	WeakTipPool

	// DroppedTipPool represents a pool of blockMetadataStorage that are supposed to be dropped.
	DroppedTipPool
)
