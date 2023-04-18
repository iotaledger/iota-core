package permanent

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

const (
	sybilProtectionPrefix byte = iota
)

type Permanent struct {
	dbConfig           database.Config
	store              kvstore.KVStore
	storeHealthTracker *kvstore.StoreHealthTracker

	settings    *Settings
	commitments *Commitments

	sybilProtection kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(baseDir *utils.Directory, dbConfig database.Config) *Permanent {
	store, err := database.StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}

	// TODO: What do we do if the DB is corrupted?
	storeHealthTracker, err := kvstore.NewStoreHealthTracker(store, dbConfig.PrefixHealth, dbConfig.Version, nil)
	if err != nil {
		panic(err)
	}
	if err = storeHealthTracker.MarkCorrupted(); err != nil {
		panic(err)
	}

	return &Permanent{
		store:              store,
		storeHealthTracker: storeHealthTracker,

		settings:        NewSettings(baseDir.Path("settings.bin")),
		commitments:     NewCommitments(baseDir.Path("commitments.bin")),
		sybilProtection: lo.PanicOnErr(store.WithExtendedRealm([]byte{sybilProtectionPrefix})),
	}
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
		// TODO: introduce logger? m.logger.LogError("dbDirectorySize failed for %s: %w", m.permanentBaseDir, err)
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

	if err := p.storeHealthTracker.MarkHealthy(); err != nil {
		panic(err)
	}

	err := p.store.Close()
	if err != nil {
		panic(err)
	}
}
