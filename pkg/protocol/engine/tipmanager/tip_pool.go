package tipmanager

// TipPool represents a pool of blocks that are treated in a certain way by the tip selection strategy.
type TipPool uint8

const (
	// UndefinedTipPool is the zero value of TipPool.
	UndefinedTipPool TipPool = iota

	// StrongTipPool represents a pool of blocks that are supposed to be referenced through strong parents.
	StrongTipPool

	// WeakTipPool represents a pool of blocks that are supposed to be referenced through weak parents.
	WeakTipPool

	// DroppedTipPool represents a pool of blocks that are supposed to be ignored by the tip selection strategy.
	DroppedTipPool
)

// Max returns the maximum of the two TipPools.
func (t TipPool) Max(other TipPool) TipPool {
	if t > other {
		return t
	}

	return other
}

// String returns a human-readable representation of the TipPool.
func (t TipPool) String() string {
	switch t {
	case StrongTipPool:
		return "StrongTipPool"
	case WeakTipPool:
		return "WeakTipPool"
	case DroppedTipPool:
		return "DroppedTipPool"
	default:
		return "UndefinedTipPool"
	}
}
