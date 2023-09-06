package permanent

import (
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota.go/v4/api"
)

func WithEpochBasedProviderOptions(opts ...options.Option[api.EpochBasedProvider]) options.Option[Permanent] {
	return func(p *Permanent) {
		p.optsEpochBasedProvider = append(p.optsEpochBasedProvider, opts...)
	}
}
