package requesthandler

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/requesthandler/cache"
)

// RequestHandler contains the logic to handle api requests.
type RequestHandler struct {
	workerPool *workerpool.WorkerPool

	cache    *cache.Cache
	protocol *protocol.Protocol

	optsCacheMaxSize int
}

func New(p *protocol.Protocol, opts ...options.Option[RequestHandler]) *RequestHandler {
	return options.Apply(&RequestHandler{
		workerPool:       p.Workers.CreatePool("RequestHandler"),
		protocol:         p,
		optsCacheMaxSize: 50 << 20, // 50MB
	}, opts, func(r *RequestHandler) {
		r.cache = cache.NewCache(r.optsCacheMaxSize)
	})
}

// Shutdown shuts down the block issuer.
func (r *RequestHandler) Shutdown() {
	r.workerPool.Shutdown()
	r.workerPool.ShutdownComplete.Wait()
}

func WithCacheMaxSizeOptions(size int) options.Option[RequestHandler] {
	return func(r *RequestHandler) {
		r.optsCacheMaxSize = size
	}
}
