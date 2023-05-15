package promise

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/runtime/options"
)

type SetMutations[T comparable] struct {
	RemovedElements *advancedset.AdvancedSet[T]
	AddedElements   *advancedset.AdvancedSet[T]
}

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

func (s *SetMutations[T]) IsEmpty() bool {
	return s.RemovedElements.IsEmpty() && s.AddedElements.IsEmpty()
}

func WithAddedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.AddedElements = elements
	}
}

func WithRemovedElements[T comparable](elements *advancedset.AdvancedSet[T]) options.Option[SetMutations[T]] {
	return func(args *SetMutations[T]) {
		args.RemovedElements = elements
	}
}
