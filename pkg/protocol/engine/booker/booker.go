package booker

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

type Booker interface {
	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
