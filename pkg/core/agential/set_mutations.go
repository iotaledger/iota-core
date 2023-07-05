package agential

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/options"
)

// SetReceptorMutations represents an atomic set of mutations that can be applied to a SetReceptor.
type SetReceptorMutations[T comparable] struct {
	// RemovedElements are the elements that are supposed to be removed.
	RemovedElements *advancedset.AdvancedSet[T]

	// AddedElements are the elements that are supposed to be added.
	AddedElements *advancedset.AdvancedSet[T]
}

// NewSetReceptorMutations creates a new SetReceptorMutations instance.
func NewSetReceptorMutations[T comparable](opts ...options.Option[SetReceptorMutations[T]]) *SetReceptorMutations[T] {
	return options.Apply(new(SetReceptorMutations[T]), opts, func(s *SetReceptorMutations[T]) {
		if s.RemovedElements == nil {
			s.RemovedElements = advancedset.New[T]()
		}

		if s.AddedElements == nil {
			s.AddedElements = advancedset.New[T]()
		}
	})
}

// IsEmpty returns true if the SetReceptorMutations instance is empty.
func (s *SetReceptorMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

// WithAddedElements is an option that can be used to set the added elements of a SetReceptorMutations instance.
func WithAddedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetReceptorMutations[T]] {
	return func(args *SetReceptorMutations[T]) {
		args.AddedElements = elements
	}
}

// WithRemovedElements is an option that can be used to set the removed elements of a SetReceptorMutations instance.
func WithRemovedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetReceptorMutations[T]] {
	return func(args *SetReceptorMutations[T]) {
		args.RemovedElements = elements
	}
}
