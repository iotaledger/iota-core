package permanent

import (
	"os"

	"github.com/iotaledger/hive.go/kvstore"
	hivedb "github.com/iotaledger/hive.go/kvstore/database"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/iota-core/pkg/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

const (
	permanentDirName           = "permanent"
	sybilProtectionPrefix byte = iota
)

type Permanent struct {
	dir              *utils.Directory
	permanentStorage kvstore.KVStore

	Settings    *Settings
	Commitments *Commitments

	sybilProtection kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(dir *utils.Directory, version database.Version, dbEngine hivedb.Engine) *Permanent {
	p := &Permanent{
		Settings:    NewSettings(dir.Path("settings.bin")),
		Commitments: NewCommitments(dir.Path("commitments.bin")),
	}

	permanentStore, err := database.StoreWithDefaultSettings(dir.Path(permanentDirName), true, dbEngine)
	if err != nil {
		panic(err)
	}
	p.permanentStorage = permanentStore

	if err = database.CheckVersion(p.permanentStorage, version); err != nil {
		panic(err)
	}

	p.sybilProtection = lo.PanicOnErr(p.permanentStorage.WithExtendedRealm([]byte{sybilProtectionPrefix}))

	return p
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
	size, err := ioutils.FolderSize(p.dir.Path(permanentDirName))
	if err != nil {
		// TODO: introduce logger? m.logger.LogError("dbDirectorySize failed for %s: %w", m.permanentBaseDir, err)
		return 0
	}

	for _, file := range []string{p.Settings.FilePath(), p.Commitments.FilePath()} {
		s, err := fileSize(file)
		if err != nil {
			panic(err)
		}
		size += s
	}

	return size
}

func (p *Permanent) Shutdown() {
	err := p.permanentStorage.Close()
	if err != nil {
		panic(err)
	}
}

func fileSize(path string) (int64, error) {
	s, err := os.Stat(path)
	if err != nil {
		return 0, err
	}

	return s.Size(), nil
}
