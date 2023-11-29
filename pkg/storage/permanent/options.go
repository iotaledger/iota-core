package permanent

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithEpochBasedProviderOptions(opts ...options.Option[iotago.EpochBasedProvider]) options.Option[Permanent] {
	return func(p *Permanent) {
		p.optsEpochBasedProvider = append(p.optsEpochBasedProvider, opts...)
	}
}
