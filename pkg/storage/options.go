package storage

import (
	"time"

	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/options"
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

func WithPrunableManagerOptions(opts ...options.Option[prunable.SlotManager]) options.Option[Storage] {
	return func(s *Storage) {
		s.optsPrunableManagerOptions = append(s.optsPrunableManagerOptions, opts...)
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
