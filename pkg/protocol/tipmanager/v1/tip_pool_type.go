package v1

type TipPoolType uint8

const (
	UndefinedTipPool TipPoolType = iota

	StrongTipPool

	WeakTipPool

	DroppedTipPool
)
