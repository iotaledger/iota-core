package agential

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/options"
)

// SetMutations represents an atomic set of mutations that can be applied to a SetReceptor.
type SetMutations[T comparable] struct {
	// RemovedElements are the elements that are supposed to be removed.
	RemovedElements *advancedset.AdvancedSet[T]

	// AddedElements are the elements that are supposed to be added.
	AddedElements *advancedset.AdvancedSet[T]
}

// NewSetMutations creates a new SetMutations instance.
func NewSetMutations[T comparable](opts ...options.Option[SetMutations[T]]) *SetMutations[T] {
	return options.Apply(new(SetMutations[T]), opts, func(s *SetMutations[T]) {
		if s.RemovedElements == nil {
			s.RemovedElements = advancedset.New[T]()
		}

		if s.AddedElements == nil {
			s.AddedElements = advancedset.New[T]()
		}
	})
}

// IsEmpty returns true if the SetMutations instance is empty.
func (s *SetMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

// WithAddedElements is an option that can be used to set the added elements of a SetMutations instance.
func WithAddedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.AddedElements = elements
	}
}

// WithRemovedElements is an option that can be used to set the removed elements of a SetMutations instance.
func WithRemovedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.RemovedElements = elements
	}
}
