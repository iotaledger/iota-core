package spammer

import (
	"fmt"

	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/syncutils"
)

var (
	ErrFailPostBlock       = ierrors.New("failed to post block")
	ErrFailSendDataBlock   = ierrors.New("failed to send a data block")
	ErrFailGetReferences   = ierrors.New("failed to get references")
	ErrTransactionIsNil    = ierrors.New("provided transaction is nil")
	ErrBlockIsNil          = ierrors.New("provided block is nil")
	ErrFailToPrepareBatch  = ierrors.New("custom conflict batch could not be prepared")
	ErrInsufficientClients = ierrors.New("insufficient clients to send conflicts")
	ErrInputsNotSolid      = ierrors.New("not all inputs are solid")
	ErrFailPrepareBlock    = ierrors.New("failed to prepare block")
)

// ErrorCounter counts errors that appeared during the spam,
// as during the spam they are ignored and allows to print the summary (might be useful for debugging).
type ErrorCounter struct {
	errorsMap       map[error]*atomic.Int64
	errInTotalCount *atomic.Int64
	mutex           syncutils.RWMutex
}

func NewErrorCount() *ErrorCounter {
	e := &ErrorCounter{
		errorsMap:       make(map[error]*atomic.Int64),
		errInTotalCount: atomic.NewInt64(0),
	}

	return e
}

func (e *ErrorCounter) CountError(err error) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	// check if error is already in the map
	if _, ok := e.errorsMap[err]; !ok {
		e.errorsMap[err] = atomic.NewInt64(0)
	}
	e.errInTotalCount.Add(1)
	e.errorsMap[err].Add(1)
}

func (e *ErrorCounter) GetTotalErrorCount() int64 {
	return e.errInTotalCount.Load()
}

func (e *ErrorCounter) GetErrorsSummary() string {
	if len(e.errorsMap) == 0 {
		return "No errors encountered"
	}
	blk := "Errors encountered during spam:\n"
	for key, value := range e.errorsMap {
		blk += fmt.Sprintf("%s: %d\n", key.Error(), value.Load())
	}

	return blk
}
