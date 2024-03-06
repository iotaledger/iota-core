package clonablesql

import (
	"path/filepath"

	copydir "github.com/otiai10/copy"
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/db"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/sql"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

// ClonableSQLiteDatabase is a wrapper around a LockableGormDB database (SQLite) that allows to clone it.
type ClonableSQLiteDatabase struct {
	logger       log.Logger
	directory    string
	filename     string
	errorHandler func(error)

	database *LockableGormDB
}

// NewClonableSQLiteDatabase creates a new ClonableSQLDatabase instance.
func NewClonableSQLiteDatabase(logger log.Logger, directory string, filename string, errorHandler func(error)) *ClonableSQLiteDatabase {
	db := &ClonableSQLiteDatabase{
		logger:       logger,
		directory:    directory,
		filename:     filename,
		errorHandler: errorHandler,
	}

	if err := db.openDatabase(); err != nil {
		panic(err)
	}

	return db
}

// openDatabase opens the SQLite database.
func (p *ClonableSQLiteDatabase) openDatabase() error {
	baseDir := utils.NewDirectory(p.directory, true)

	sqliteDatabase, _, err := sql.New(
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

	p.database = NewLockableGormDB(sqliteDatabase)

	return nil
}

// Clone creates a new ClonableSQLDatabase instance by copying the storage on disk to a new location.
func Clone(logger log.Logger, source *ClonableSQLiteDatabase, directory string, filename string, errorHandler func(error)) (*ClonableSQLiteDatabase, error) {
	// Lock database so that nobody can try to use or open it while cloning.
	source.database.LockAccess()
	defer source.database.UnlockAccess()

	// Close forked database before copying its contents. All necessary locks are already acquired.
	if err := source.database.CloseWithoutLocking(); err != nil {
		return nil, ierrors.Wrap(err, "failed to close source database before cloning")
	}

	// Copy the database on disk to new location.
	if err := copydir.Copy(source.directory, directory); err != nil {
		return nil, ierrors.Wrapf(err, "failed to copy SQLite database directory to new path: %s", directory)
	}

	// Reopen the source database.
	if err := source.openDatabase(); err != nil {
		return nil, ierrors.Wrap(err, "failed to reopen source database after cloning")
	}

	return NewClonableSQLiteDatabase(logger, directory, filename, errorHandler), nil
}

// ExecDBFunc executes a function with the DB as argument.
// The DB is locked during the execution of the function.
func (p *ClonableSQLiteDatabase) ExecDBFunc() func(func(*gorm.DB) error) error {
	return p.database.ExecDBFunc
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
	if err := p.database.Close(); err != nil {
		p.errorHandler(ierrors.Wrapf(err, "failed to close clonable SQL database: %s", filepath.Join(p.directory, p.filename)))
	}
}
