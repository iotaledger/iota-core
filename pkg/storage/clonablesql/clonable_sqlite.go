package clonablesql

import (
	"path/filepath"

	copydir "github.com/otiai10/copy"
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/sql"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

// ClonableSQLiteDatabase is a wrapper around a gorm database (SQLite) that allows to clone it.
type ClonableSQLiteDatabase struct {
	logger       log.Logger
	directory    string
	filename     string
	errorHandler func(error)

	accessMutex *syncutils.StarvingMutex
	database    *gorm.DB
}

// NewClonableSQLiteDatabase creates a new ClonableSQLDatabase instance.
func NewClonableSQLiteDatabase(logger log.Logger, directory string, filename string, errorHandler func(error)) *ClonableSQLiteDatabase {
	db := &ClonableSQLiteDatabase{
		logger:       logger,
		directory:    directory,
		filename:     filename,
		errorHandler: errorHandler,
		accessMutex:  syncutils.NewStarvingMutex(),
	}

	// we don't need to lock the database here, as the ClonableSQLiteDatabase is not used by anyone else yet.
	if err := db.openDatabaseWithoutLocking(); err != nil {
		panic(err)
	}

	return db
}

// openDatabaseWithoutLocking opens the SQLite database.
func (p *ClonableSQLiteDatabase) openDatabaseWithoutLocking() error {
	baseDir := utils.NewDirectory(p.directory, true)

	gormDB, _, err := sql.New(
		p.logger,
		sql.DatabaseParameters{
			Engine:   db.EngineSQLite,
			Path:     baseDir.Path(),
			Filename: p.filename,
		},
		true,
		[]db.Engine{db.EngineSQLite},
	)
	if err != nil {
		return ierrors.Wrapf(err, "failed to create/open SQLite database: %s", filepath.Join(baseDir.Path(), p.filename))
	}

	p.database = gormDB

	return nil
}

// closeWithoutLocking closes the SQLite database without locking access to it.
func (p *ClonableSQLiteDatabase) closeDatabaseWithoutLocking() error {
	gormDB, err := p.database.DB()
	if err != nil {
		return ierrors.Wrapf(err, "failed to get SQLite database: %s", filepath.Join(p.directory, p.filename))
	}

	return gormDB.Close()
}

// Clone creates a new ClonableSQLDatabase instance by copying the SQLite database to a new location.
func Clone(logger log.Logger, source *ClonableSQLiteDatabase, directory string, filename string, errorHandler func(error)) (*ClonableSQLiteDatabase, error) {
	// Lock database so that nobody can try to use or open it while cloning.
	// After the lock is acquired, all ongoing database operations are paused.
	source.accessMutex.Lock()
	defer source.accessMutex.Unlock()

	// Close forked database before copying its contents. All necessary locks are already acquired.
	if err := source.closeDatabaseWithoutLocking(); err != nil {
		return nil, ierrors.Wrap(err, "failed to close source SQLite database before cloning")
	}

	// Copy the database on disk to new location.
	if err := copydir.Copy(source.directory, directory); err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy SQLite database directory to new path: %s", directory)
	}

	// Reopen the source database, the lock is already acquired above.
	if err := source.openDatabaseWithoutLocking(); err != nil {
		return nil, ierrors.Wrap(err, "failed to reopen source SQLite database after cloning")
	}

	// Create a new ClonableSQLiteDatabase instance for the clone.
	return NewClonableSQLiteDatabase(logger, directory, filename, errorHandler), nil
}

// ExecDBFunc executes a function with the DB as argument.
// The DB is locked during the execution of the function.
func (p *ClonableSQLiteDatabase) ExecDBFunc() func(func(*gorm.DB) error) error {
	// we need to return a function pointer that resolves the database exec call to the latest
	// instance of the database with every call to the inner exec func.
	return func(dbFunc func(*gorm.DB) error) error {
		resolveDatabaseExecFunc := func(dbFuncInner func(*gorm.DB) error) error {
			// Lock the database so that nobody uses it while cloning or closing.
			p.accessMutex.RLock()
			defer p.accessMutex.RUnlock()

			return dbFuncInner(p.database)
		}

		return resolveDatabaseExecFunc(dbFunc)
	}
}

// Size returns the size of the underlying database.
func (p *ClonableSQLiteDatabase) Size() int64 {
	folderSize, err := ioutils.FolderSize(p.directory)
	if err != nil {
		p.errorHandler(ierrors.Wrapf(err, "get folder size failed for %s", p.directory))
	}

	return folderSize
}

// Shutdown closes the database.
func (p *ClonableSQLiteDatabase) Shutdown() {
	p.accessMutex.Lock()
	defer p.accessMutex.Unlock()

	if err := p.closeDatabaseWithoutLocking(); err != nil {
		p.errorHandler(ierrors.Wrap(err, "failed to close SQLite database"))
	}
}
