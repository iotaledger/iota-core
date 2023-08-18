package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
)

func trapdoor(currentValue *Chain, newValue *Chain) *Chain {
	return lo.Cond(currentValue != nil, currentValue, newValue)
}

func inheritTo(target reactive.Variable[bool]) func(bool, bool) {
	return func(_, newValue bool) {
		target.Set(newValue)
	}
}
