package pruning

import (
	"github.com/iotaledger/hive.go/runtime/module"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Pruning is an interface for pruning the database.
// This should live in engine?
type Pruning interface {
	PruneDatabaseBySize(targetSizeBytes ...int64) error
	PruneDatabaseByDepth(depth iotago.SlotIndex) error
	PruneDatabaseByIndex(index iotago.SlotIndex) error

	IsPruning() bool

	module.Interface
}
