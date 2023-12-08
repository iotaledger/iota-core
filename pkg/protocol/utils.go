package protocol

import (
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
)

func processRequest(workerPool *workerpool.WorkerPool, requestFunc func() error, logger log.Logger, loggerArgs ...any) {
	workerPool.Submit(func() {
		if err := requestFunc(); err != nil {
			logger.LogDebug("failed to answer request", append(loggerArgs, "err", err)...)
		} else {
			logger.LogTrace("answered request", loggerArgs...)
		}
	})
}
