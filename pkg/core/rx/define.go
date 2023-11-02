package rx

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
)

func WithNonEmptyValue[S comparable](source reactive.Variable[S], dependencyReceiver func(S) func()) (unsubscribe func()) {
	return source.OnUpdateWithContext(func(_, parent S, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if parent != *new(S) {
			unsubscribeOnParentUpdate(func() (unsubscribe func()) {
				return dependencyReceiver(parent)
			})
		}
	})
}

func With1Dependency[S comparable](source reactive.Variable[S]) func(dependencyReceiver ...func(S) func()) (unsubscribe func()) {
	return func(dependencyReceiver ...func(S) func()) (unsubscribe func()) {
		return source.OnUpdateWithContext(func(_, parent S, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
			if parent == *new(S) {
				return
			}

			unsubscribeOnParentUpdate(func() (unsubscribe func()) {
				unsubscribeAll := make([]func(), 0)

				for _, dependency := range dependencyReceiver {
					if unsubscribeDependency := dependency(parent); unsubscribeDependency != nil {
						unsubscribeAll = append(unsubscribeAll, unsubscribeDependency)
					}
				}

				return lo.Batch(unsubscribeAll...)
			})
		})
	}
}

func AssignValue[T comparable](target reactive.Variable[T], derivedVariable reactive.DerivedVariable[T]) func() {
	target.InheritFrom(derivedVariable)

	return derivedVariable.Unsubscribe
}

func DynamicValue1[T, S comparable](target reactive.Variable[T], definition func(S) reactive.DerivedVariable[T]) func(dependency S) func() {
	return func(dependency S) func() {
		derivedVariable := definition(dependency)
		target.InheritFrom(derivedVariable)

		return derivedVariable.Unsubscribe
	}
}

func InheritedValue[T, S comparable](target reactive.Variable[T], definition func(S) reactive.ReadableVariable[T]) func(dependency S) func() {
	return func(dependency S) func() {
		return target.InheritFrom(definition(dependency))
	}
}

func With2Dependencies[S1, S2 comparable](source1 reactive.Variable[S1], source2 reactive.Variable[S2]) func(dependencyReceivers ...func(S1, S2) func()) (unsubscribe func()) {
	return func(dependencyReceivers ...func(S1, S2) func()) func() {
		return source1.OnUpdateWithContext(func(_, source1 S1, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
			if source1 == *new(S1) {
				return
			}

			unsubscribeOnParentUpdate(func() (unsubscribe func()) {
				return source2.OnUpdateWithContext(func(_, source2 S2, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
					if source2 == *new(S2) {
						return
					}

					unsubscribeOnParentUpdate(func() (unsubscribe func()) {
						unsubscribeAll := make([]func(), 0)

						for _, dependency := range dependencyReceivers {
							if unsubscribeDependency := dependency(source1, source2); unsubscribeDependency != nil {
								unsubscribeAll = append(unsubscribeAll, unsubscribeDependency)
							}
						}

						return lo.Batch(unsubscribeAll...)
					})
				})
			})
		})
	}
}

func DynamicValue2[T, S1, S2 comparable](target reactive.Variable[T], definition func(S1, S2) reactive.DerivedVariable[T]) func(S1, S2) func() {
	return func(source1 S1, source2 S2) func() {
		derivedVariable := definition(source1, source2)

		return lo.Batch(target.InheritFrom(derivedVariable), derivedVariable.Unsubscribe)
	}
}

func StaticValue1[T, S comparable](target reactive.Variable[T], definition func(S) T) func(parent S) func() {
	return func(parent S) func() {
		target.Set(definition(parent))

		return nil
	}
}
