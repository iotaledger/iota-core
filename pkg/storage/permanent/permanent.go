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
	dbConfig database.Config
	store    kvstore.KVStore

	Settings    *Settings
	Commitments *Commitments

	sybilProtection kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(baseDir *utils.Directory, dbConfig database.Config) *Permanent {
	store, err := database.StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
	if err != nil {
		panic(err)
	}

	return &Permanent{
		store:           store,
		Settings:        NewSettings(baseDir.Path("settings.bin")),
		Commitments:     NewCommitments(baseDir.Path("commitments.bin")),
		sybilProtection: lo.PanicOnErr(store.WithExtendedRealm([]byte{sybilProtectionPrefix})),
	}
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

	settingsSize, err := p.Settings.Size()
	if err != nil {
		panic(err)
	}

	commitmentsSize, err := p.Settings.Size()
	if err != nil {
		panic(err)
	}

	return dbSize + settingsSize + commitmentsSize
}

func (p *Permanent) Shutdown() {
	if err := p.Commitments.Close(); err != nil {
		panic(err)
	}

	err := p.store.Close()
	if err != nil {
		panic(err)
	}
}
