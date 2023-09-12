package storage

import (
	"time"

	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithDBEngine(optsDBEngine hivedb.Engine) options.Option[Storage] {
	return func(s *Storage) {
		s.optsDBEngine = optsDBEngine
	}
}

func WithAllowedDBEngines(optsAllowedDBEngines []hivedb.Engine) options.Option[Storage] {
	return func(s *Storage) {
		s.optsAllowedDBEngines = optsAllowedDBEngines
	}
}

func WithBucketManagerOptions(opts ...options.Option[prunable.BucketManager]) options.Option[Storage] {
	return func(s *Storage) {
		s.optsBucketManagerOptions = append(s.optsBucketManagerOptions, opts...)
	}
}

func WithPruningDelay(optsPruningDelay iotago.EpochIndex) options.Option[Storage] {
	return func(s *Storage) {
		s.optsPruningDelay = optsPruningDelay
	}
}

func WithPruningSizeEnable(pruningSizeEnabled bool) options.Option[Storage] {
	return func(p *Storage) {
		p.optPruningSizeEnabled = pruningSizeEnabled
	}
}

func WithPruningSizeMaxTargetSizeBytes(pruningSizeTargetSizeBytes int64) options.Option[Storage] {
	return func(p *Storage) {
		p.optsPruningSizeMaxTargetSizeBytes = pruningSizeTargetSizeBytes
	}
}

func WithPruningSizeReductionPercentage(pruningSizeReductionPercentage float64) options.Option[Storage] {
	return func(p *Storage) {
		p.optsPruningSizeReductionPercentage = pruningSizeReductionPercentage
	}
}

func WithPruningSizeCooldownTime(cooldown time.Duration) options.Option[Storage] {
	return func(p *Storage) {
		p.optsPruningSizeCooldownTime = cooldown
	}
}

func WithPermanentOptions(opts ...options.Option[permanent.Permanent]) options.Option[Storage] {
	return func(s *Storage) {
		s.optsPermanent = append(s.optsPermanent, opts...)
	}
}
