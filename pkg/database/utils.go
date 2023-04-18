package database

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

var healthKey = []byte("bucket_health")

var dbVersionKey = []byte("db_version")

// indexToRealm converts an baseIndex to a realm with some shifting magic.
func indexToRealm(index iotago.SlotIndex) kvstore.Realm {
	return []byte{
		byte(0xff & index),
		byte(0xff & (index >> 8)),
		byte(0xff & (index >> 16)),
		byte(0xff & (index >> 24)),
		byte(0xff & (index >> 32)),
		byte(0xff & (index >> 40)),
		byte(0xff & (index >> 48)),
		byte(0xff & (index >> 54)),
	}
}

func dbPathFromIndex(base string, index iotago.SlotIndex) string {
	return filepath.Join(base, strconv.FormatInt(int64(index), 10))
}

type dbInstanceFileInfo struct {
	baseIndex iotago.SlotIndex
	path      string
}

func getSortedDBInstancesFromDisk(baseDir string) (dbInfos []*dbInstanceFileInfo) {
	files, err := os.ReadDir(baseDir)
	if err != nil {
		panic(err)
	}

	files = lo.Filter(files, func(e os.DirEntry) bool { return e.IsDir() })
	dbInfos = lo.Map(files, func(e os.DirEntry) *dbInstanceFileInfo {
		atoi, convErr := strconv.Atoi(e.Name())
		if convErr != nil {
			return nil
		}
		return &dbInstanceFileInfo{
			baseIndex: iotago.SlotIndex(atoi),
			path:      filepath.Join(baseDir, e.Name()),
		}
	})
	dbInfos = lo.Filter(dbInfos, func(info *dbInstanceFileInfo) bool { return info != nil })

	sort.Slice(dbInfos, func(i, j int) bool {
		return dbInfos[i].baseIndex > dbInfos[j].baseIndex
	})

	return dbInfos
}

func dbPrunableDirectorySize(base string, index iotago.SlotIndex) (int64, error) {
	return dbDirectorySize(dbPathFromIndex(base, index))
}

func dbDirectorySize(path string) (int64, error) {
	return ioutils.FolderSize(path)
}

// CheckVersion checks whether the database is compatible with the current schema version.
// also automatically sets the version if the database is new.
func CheckVersion(db kvstore.KVStore, version Version) error {
	entry, err := db.Get(dbVersionKey)
	if errors.Is(err, kvstore.ErrKeyNotFound) {
		// set the version in an empty DB
		return db.Set(dbVersionKey, lo.PanicOnErr(version.Bytes()))
	}
	if err != nil {
		return err
	}
	if len(entry) == 0 {
		return errors.Errorf("no database version was persisted")
	}
	var storedVersion Version
	if _, err := storedVersion.FromBytes(entry); err != nil {
		return err
	}
	if storedVersion != version {
		return errors.Errorf("incompatible database versions: supported version: %d, version of database: %d", version, storedVersion)
	}
	return nil
}
