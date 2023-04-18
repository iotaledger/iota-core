package storage

import (
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/options"
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

func WithLogger(log *logger.Logger) options.Option[Storage] {
	return func(s *Storage) {
		s.optsLogger = log
	}
}

// TODO: adjust for bucket store
// func WithStorageDatabaseManagerOptions(opts ...options.Option[storage.Storage]) options.Option[Protocol] {
// 	return func(p *Protocol) {
// 		p.optsStorageOptions = append(p.optsStorageOptions, opts...)
// 	}
// }
