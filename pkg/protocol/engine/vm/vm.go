package vm

import (
	"iota-core/pkg/iotago"
)

type VM func(transaction iotago.Transaction, inputs []iotago.Output, gasLimit ...uint64) (outputs []iotago.Output, err error)
