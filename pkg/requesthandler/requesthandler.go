package requesthandler

import (
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol"
)

// RequestHandler contains the logic to handle api requests.
type RequestHandler struct {
	workerPool *workerpool.WorkerPool

	protocol *protocol.Protocol
}

func New(p *protocol.Protocol) *RequestHandler {
	return &RequestHandler{
		workerPool: p.Workers.CreatePool("BlockHandler"),
		protocol:   p,
	}
}

// Shutdown shuts down the block issuer.
func (r *RequestHandler) Shutdown() {
	r.workerPool.Shutdown()
	r.workerPool.ShutdownComplete.Wait()
}
