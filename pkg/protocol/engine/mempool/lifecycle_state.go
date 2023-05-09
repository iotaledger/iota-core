package mempool

type LifecycleState interface {
	IsSolid() bool

	OnSolid(func())

	IsExecuted() bool

	OnExecuted(func())

	IsInvalid() bool

	OnInvalid(func(error))

	IsBooked() bool

	OnBooked(func())
}
