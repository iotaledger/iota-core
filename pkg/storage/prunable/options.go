package prunable

import (
	"github.com/iotaledger/hive.go/runtime/options"
)

// WithMaxOpenDBs sets the maximum concurrently open DBs.
func WithMaxOpenDBs(optsMaxOpenDBs int) options.Option[BucketManager] {
	return func(m *BucketManager) {
		m.optsMaxOpenDBs = optsMaxOpenDBs
	}
}
