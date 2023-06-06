package permanent

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

const (
	sybilProtectionPrefix byte = iota
	attestationsPrefix
	ledgerPrefix
)

type Permanent struct {
	dbConfig      database.Config
	store         kvstore.KVStore
	healthTracker *kvstore.StoreHealthTracker
	errorHandler  func(error)

	settings    *Settings
	commitments *Commitments

	sybilProtection kvstore.KVStore
	attestations    kvstore.KVStore
	ledger          kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(baseDir *utils.Directory, dbConfig database.Config, errorHandler func(error), opts ...options.Option[Permanent]) *Permanent {
	return options.Apply(&Permanent{
		errorHandler: errorHandler,
		settings:     NewSettings(baseDir.Path("settings.bin")),
	}, opts, func(p *Permanent) {
		p.commitments = NewCommitments(baseDir.Path("commitments.bin"), p.settings.API)

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

		p.sybilProtection = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{sybilProtectionPrefix}))
		p.attestations = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{attestationsPrefix}))
		p.ledger = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{ledgerPrefix}))
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

// Ledger returns the ledger storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Ledger(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.ledger
	}

	return lo.PanicOnErr(p.ledger.WithExtendedRealm(optRealm))
}

// Size returns the size of the permanent storage.
func (p *Permanent) Size() int64 {
	dbSize, err := ioutils.FolderSize(p.dbConfig.Directory)
	if err != nil {
		p.errorHandler(errors.Wrapf(err, "dbDirectorySize failed for %s", p.dbConfig.Directory))
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
