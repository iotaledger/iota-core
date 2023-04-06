package database

import (
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/runtime/options"
)

// WithGranularity sets the granularity of the DB instances (i.e. how many buckets/slots are stored in one DB).
// It thus also has an impact on how fine-grained buckets/slots can be pruned.
func WithGranularity(granularity int64) options.Option[Manager] {
	return func(m *Manager) {
		m.optsGranularity = granularity
	}
}

// WithBaseDir sets the base directory to store the DB to disk.
func WithBaseDir(baseDir string) options.Option[Manager] {
	return func(m *Manager) {
		m.optsBaseDir = baseDir
	}
}

// WithMaxOpenDBs sets the maximum concurrently open DBs.
func WithMaxOpenDBs(optsMaxOpenDBs int) options.Option[Manager] {
	return func(m *Manager) {
		m.optsMaxOpenDBs = optsMaxOpenDBs
	}
}

func WithDBEngine(optsDBEngine hivedb.Engine) options.Option[Manager] {
	return func(m *Manager) {
		m.optsDBEngine = optsDBEngine
	}
}

func WithAllowedDBEngines(optsAllowedDBEngines []hivedb.Engine) options.Option[Manager] {
	return func(m *Manager) {
		m.optsAllowedDBEngines = optsAllowedDBEngines
	}
}
