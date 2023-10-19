package booker

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

type Booker interface {
	// Reset resets the Booker to its empty state after the last commitment.
	Reset()

	module.Interface
}
