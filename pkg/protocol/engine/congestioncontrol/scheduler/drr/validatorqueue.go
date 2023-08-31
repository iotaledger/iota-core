package drr

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"go.uber.org/atomic"
)

type ValidatorQueue struct {
	accountID iotago.AccountID
	submitted *shrinkingmap.ShrinkingMap[iotago.BlockID, *blocks.Block]
	size      atomic.Int64
}

func NewValidatorQueue(accountID iotago.AccountID) *ValidatorQueue {
	return &ValidatorQueue{
		accountID: accountID,
		submitted: shrinkingmap.New[iotago.BlockID, *blocks.Block](),
	}
}

func (q *ValidatorQueue) Size() int {
	if q == nil {
		return 0
	}

	return int(q.size.Load())
}

func (q *ValidatorQueue) AccountID() iotago.AccountID {
	return q.accountID
}

func (q *ValidatorQueue) Submit(block *blocks.Block) bool {
	if blkAccountID := block.ProtocolBlock().IssuerID; q.accountID != blkAccountID {
		panic(fmt.Sprintf("issuerqueue: queue issuer ID(%x) and issuer ID(%x) does not match.", q.accountID, blkAccountID))
	}

	if _, submitted := q.submitted.Get(block.ID()); submitted {
		return false
	}

	q.submitted.Set(block.ID(), block)
	q.size.Inc()

	return true
}

func (q *ValidatorQueue) Unsubmit(block *blocks.Block) bool {
	if _, submitted := q.submitted.Get(block.ID()); !submitted {
		return false
	}

	q.submitted.Delete(block.ID())
	q.size.Dec()

	return true
}
