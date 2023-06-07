package notarization

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

type Notarization interface {
	// IsBootstrapped returns if notarization finished committing all pending slots up to the current acceptance time.
	IsBootstrapped() bool

	module.Interface
}
