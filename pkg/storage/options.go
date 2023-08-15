package storage

import (
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

func WithPrunableManagerOptions(opts ...options.Option[prunable.PrunableSlotManager]) options.Option[Storage] {
	return func(s *Storage) {
		s.optsPrunableManagerOptions = append(s.optsPrunableManagerOptions, opts...)
	}
}

func WithPruningDelay(optsPruningDelay iotago.EpochIndex) options.Option[Storage] {
	return func(s *Storage) {
		s.optsPruningDelay = optsPruningDelay
	}
}

func WithPruningSizeMaxTargetSizeBytes(pruningSizeTargetSizeBytes int64) options.Option[Storage] {
	return func(p *Storage) {
		p.optPruningSizeMaxTargetSizeBytes = pruningSizeTargetSizeBytes
	}
}

func WithStartPruningSizeThresholdPercentage(pruningSizeThresholdPercentage float64) options.Option[Storage] {
	return func(p *Storage) {
		p.optStartPruningSizeThresholdPercentage = pruningSizeThresholdPercentage
	}
}

func WithStopPruningSizeThresholdPercentage(pruningSizeThresholdPercentage float64) options.Option[Storage] {
	return func(p *Storage) {
		p.optStopPruningSizeThresholdPercentage = pruningSizeThresholdPercentage
	}
}
