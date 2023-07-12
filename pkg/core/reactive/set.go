package reactive

import (
	"github.com/iotaledger/hive.go/ds"
)

// region Set ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// Set is a reactive Set implementation that allows consumers to subscribe to its changes.
type Set[ElementType comparable] interface {
	// WriteableSet imports the write methods of the Set interface.
	ds.WriteableSet[ElementType]

	// ReadableSet imports the read methods of the Set interface.
	ReadableSet[ElementType]
}

// NewSet creates a new Set with the given elements.
func NewSet[T comparable](elements ...T) Set[T] {
	return newSet(elements...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region ReadableSet //////////////////////////////////////////////////////////////////////////////////////////////////

// ReadableSet is a reactive Set implementation that allows consumers to subscribe to its value.
type ReadableSet[ElementType comparable] interface {
	// OnUpdate registers the given callback that is triggered when the value changes.
	OnUpdate(callback func(appliedMutations ds.SetMutations[ElementType]), triggerWithInitialZeroValue ...bool) (unsubscribe func())

	// ReadableSet imports the read methods of the Set interface.
	ds.ReadableSet[ElementType]
}

// NewReadableSet creates a new ReadableSet with the given elements.
func NewReadableSet[ElementType comparable](elements ...ElementType) ReadableSet[ElementType] {
	return newReadableSet(elements...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region DerivedSet ///////////////////////////////////////////////////////////////////////////////////////////////////

// DerivedSet is a reactive Set implementation that allows consumers to subscribe to its changes and that inherits its
// elements from other sets.
type DerivedSet[ElementType comparable] interface {
	Set[ElementType]

	InheritFrom(sources ...ReadableSet[ElementType]) (unsubscribe func())
}

// NewDerivedSet creates a new DerivedSet with the given elements.
func NewDerivedSet[ElementType comparable]() DerivedSet[ElementType] {
	return newDerivedSet[ElementType]()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
