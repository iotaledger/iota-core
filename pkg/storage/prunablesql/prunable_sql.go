package prunablesql

import (
	copydir "github.com/otiai10/copy"
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/sql"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

type PrunableSQL struct {
	errorHandler func(error)

	databaseInstance *LockableGormDB
	directory        string
}

func New(parentLogger log.Logger, directory string, errorHandler func(error)) *PrunableSQL {
	baseDir := utils.NewDirectory(directory, true)

	sqlDatabase, _, err := sql.New(
		parentLogger.NewChildLogger("sql"),
		sql.DatabaseParameters{
			Engine:   db.EngineSQLite,
			Path:     baseDir.Path(),
			Filename: "tx_retainer.db",
		},
		true,
		[]db.Engine{db.EngineSQLite},
	)
	if err != nil {
		panic(err)
	}

	database := NewLockableGormDB(sqlDatabase)

	return &PrunableSQL{
		errorHandler:     errorHandler,
		databaseInstance: database,
		directory:        baseDir.Path(),
	}
}

func Clone(parentLogger log.Logger, source *PrunableSQL, directory string, errorHandler func(error)) (*PrunableSQL, error) {
	// Lock semi-permanent DB and prunable slot store so that nobody can try to use or open them while cloning.
	source.databaseInstance.LockAccess()
	defer source.databaseInstance.UnlockAccess()

	// Close forked prunable storage before copying its contents. All necessary locks are already acquired.
	source.databaseInstance.CloseWithoutLocking()

	// Copy the storage on disk to new location.
	if err := copydir.Copy(source.directory, directory); err != nil {
		return nil, ierrors.Wrap(err, "failed to copy prunable storage directory to new storage path")
	}

	return New(parentLogger, directory, errorHandler), nil
}

//func (p *PrunableSQL) RestoreFromDisk() (lastPrunedEpoch iotago.EpochIndex) {
//
//	return
//}

func (p *PrunableSQL) ExecFunc() func(func(*gorm.DB) error) error {
	return p.databaseInstance.ExecDBFunc
}

func (p *PrunableSQL) Size() int64 {
	semiSize, err := ioutils.FolderSize(p.directory)
	if err != nil {
		p.errorHandler(ierrors.Wrapf(err, "get folder size failed for %s", p.directory))
	}

	return semiSize
}

func (p *PrunableSQL) Shutdown() {
	p.databaseInstance.Close()
}
