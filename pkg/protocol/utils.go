package protocol

import (
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
)

// submitLoggedRequest is a generic utility function that submits a request to the given worker pool logging the result.
func submitLoggedRequest(workerPool *workerpool.WorkerPool, processRequest func() error, logger log.Logger, loggerArgs ...any) {
	workerPool.Submit(func() {
		if err := processRequest(); err != nil {
			logger.LogDebug("failed to answer request", append(loggerArgs, "err", err)...)
		} else {
			logger.LogTrace("answered request", loggerArgs...)
		}
	})
}
