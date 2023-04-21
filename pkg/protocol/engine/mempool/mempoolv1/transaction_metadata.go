package mempoolv1

import (
	"sync"

	"iota-core/pkg/protocol/engine/ledger"
	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/ds/advancedset"
)

type TransactionMetadata struct {
	id            mempool.TransactionID
	inputs        []*OutputMetadata
	outputs       []*OutputMetadata
	missingInputs *advancedset.AdvancedSet[ledger.OutputID]
	Transaction   mempool.Transaction

	mutex sync.RWMutex
}

func (t *TransactionMetadata) ID() mempool.TransactionID {
	return t.id
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
