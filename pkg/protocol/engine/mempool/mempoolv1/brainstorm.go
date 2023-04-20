package mempoolv1

import (
	"sync"

	"iota-core/pkg/protocol/engine/mempool"

	"github.com/iotaledger/hive.go/ds/advancedset"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Output struct {
	ID       iotago.OutputID
	Source   *Transaction
	Spenders *advancedset.AdvancedSet[*Transaction]
	Content  *mempool.Output
}

func (o *Output) IsSpent() bool {
	return o.Spenders.Size() > 0
}

func (o *Output) IsSolid() bool {
	return o.Source != nil
}

type Transaction struct {
	id            iotago.TransactionID
	missingInputs *advancedset.AdvancedSet[iotago.OutputID]
	inputs        []*Output
	outputs       []*Output
	Content       mempool.Transaction

	mutex sync.RWMutex
}

func (t *Transaction) PublishInput(index int, input *Output) bool {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.missingInputs.Delete(input.ID) {
		return false
	}

	t.inputs[index] = input

	return t.missingInputs.Size() == 0
}
