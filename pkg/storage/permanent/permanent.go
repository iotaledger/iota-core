package permanent

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/logger"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

const (
	sybilProtectionPrefix byte = iota
)

type Permanent struct {
	dbConfig      database.Config
	store         kvstore.KVStore
	healthTracker *kvstore.StoreHealthTracker

	settings    *Settings
	commitments *Commitments

	sybilProtection kvstore.KVStore

	optsLogger *logger.Logger
	logger     *logger.WrappedLogger
}

// New returns a new permanent storage instance.
func New(baseDir *utils.Directory, dbConfig database.Config, opts ...options.Option[Permanent]) *Permanent {
	return options.Apply(&Permanent{
		settings:    NewSettings(baseDir.Path("settings.bin")),
		commitments: NewCommitments(baseDir.Path("commitments.bin")),
	}, opts, func(p *Permanent) {
		p.logger = logger.NewWrappedLogger(p.optsLogger)

		var err error
		p.store, err = database.StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
		if err != nil {
			panic(err)
		}

		p.healthTracker, err = kvstore.NewStoreHealthTracker(p.store, dbConfig.PrefixHealth, dbConfig.Version, nil)
		if err != nil {
			panic(errors.Wrapf(err, "database in %s is corrupted, delete database and resync node", dbConfig.Directory))
		}
		if err = p.healthTracker.MarkCorrupted(); err != nil {
			panic(err)
		}
	})
}

func (p *Permanent) Settings() *Settings {
	return p.settings
}

func (p *Permanent) Commitments() *Commitments {
	return p.commitments
}

// SybilProtection returns the sybil protection storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) SybilProtection(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.sybilProtection
	}

	return lo.PanicOnErr(p.sybilProtection.WithExtendedRealm(optRealm))
}

// Size returns the size of the permanent storage.
func (p *Permanent) Size() int64 {
	dbSize, err := ioutils.FolderSize(p.dbConfig.Directory)
	if err != nil {
		p.logger.LogError("dbDirectorySize failed for %s: %w", p.dbConfig.Directory, err)
		return 0
	}

	settingsSize, err := p.settings.Size()
	if err != nil {
		panic(err)
	}

	commitmentsSize, err := p.settings.Size()
	if err != nil {
		panic(err)
	}

	return dbSize + settingsSize + commitmentsSize
}

func (p *Permanent) Shutdown() {
	if err := p.commitments.Close(); err != nil {
		panic(err)
	}

	if err := p.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
	if err := database.FlushAndClose(p.store); err != nil {
		panic(err)
	}
}
