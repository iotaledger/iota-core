package reactive

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/set"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/types"
)

// ReadableSet is a reactive Set implementation that allows consumers to subscribe to its value.
type ReadableSet[ElementType comparable] interface {
	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(callback func(appliedMutations set.Mutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func())

	// Readable imports the read methods of the Set interface.
	set.Readable[ElementType]
}

// NewReadableSet creates a new ReadableSet with the given elements.
func NewReadableSet[ElementType comparable](elements ...ElementType) ReadableSet[ElementType] {
	return newReadableSet(elements...)
}

// readableSet is the standard implementation of the ReadableSet interface.
type readableSet[ElementType comparable] struct {
	// updateCallbacks are the registered callbacks that are triggered when the value changes.
	updateCallbacks *shrinkingmap.ShrinkingMap[types.UniqueID, *callback[func(set.Mutations[ElementType])]]

	// uniqueUpdateID is the unique ID that is used to identify an update.
	uniqueUpdateID types.UniqueID

	// uniqueCallbackID is the unique ID that is used to identify a callback.
	uniqueCallbackID types.UniqueID

	// value is the current value of the set.
	value set.Set[ElementType]

	// valueMutex is the applyMutex that is used to synchronize the access to the value.
	valueMutex sync.RWMutex

	// Readable embeds the set.Readable interface.
	set.Readable[ElementType]
}

// newReadableSet creates a new readableSet with the given elements.
func newReadableSet[ElementType comparable](elements ...ElementType) *readableSet[ElementType] {
	setInstance := set.New[ElementType](elements...)

	return &readableSet[ElementType]{
		Readable:        setInstance.ToReadOnly(),
		value:           setInstance,
		updateCallbacks: shrinkingmap.New[types.UniqueID, *callback[func(set.Mutations[ElementType])]](),
	}
}

// OnUpdate registers the given callback to be triggered when the value of the set changes.
func (r *readableSet[ElementType]) OnUpdate(callback func(appliedMutations set.Mutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func()) {
	r.valueMutex.Lock()

	mutations := set.NewMutations[ElementType]().WithAddedElements(r)

	createdCallback := newCallback[func(set.Mutations[ElementType])](r.uniqueCallbackID.Next(), callback)
	r.updateCallbacks.Set(createdCallback.ID, createdCallback)

	// grab the lock to make sure that the callback is not executed before we have called it with the initial value.
	createdCallback.LockExecution(r.uniqueUpdateID)
	defer createdCallback.UnlockExecution()

	r.valueMutex.Unlock()

	if !mutations.IsEmpty() || lo.First(triggerWithInitialZeroValue) {
		createdCallback.Invoke(mutations)
	}

	return func() {
		r.updateCallbacks.Delete(createdCallback.ID)

		createdCallback.MarkUnsubscribed()
	}
}
