package mempool

import (
	"sync"

	"iota-core/pkg/types"

	"github.com/iotaledger/hive.go/ds/advancedset"
)

type TransactionMetadata struct {
	id            types.TransactionID
	inputs        []*OutputMetadata
	outputs       []*OutputMetadata
	missingInputs *advancedset.AdvancedSet[types.OutputID]
	Transaction   types.Transaction

	mutex sync.RWMutex
}

func (t *TransactionMetadata) IsBooked() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	return t.outputs != nil
}

func (t *TransactionMetadata) PublishInput(index int, input *OutputMetadata) (allOutputsAvailable bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.missingInputs.Delete(input.ID) {
		return false
	}

	t.inputs[index] = input

	return t.missingInputs.Size() == 0
}
