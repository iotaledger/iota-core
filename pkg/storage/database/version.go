package database

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
)

type Version byte

func (v *Version) Bytes() (bytes []byte, err error) {
	return []byte{byte(*v)}, nil
}

func (v *Version) FromBytes(bytes []byte) (consumedBytes int, err error) {
	if len(bytes) == 0 {
		return 0, errors.Errorf("not enough bytes")
	}

	*v = Version(bytes[0])
	return 1, nil
}

var dbVersionKey = []byte("db_version")

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
