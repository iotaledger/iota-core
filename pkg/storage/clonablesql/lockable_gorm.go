package clonablesql

import (
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/runtime/syncutils"
)

// LockableGormDB is a wrapper around a Gorm DB that allows to lock access to it.
type LockableGormDB struct {
	DB          *gorm.DB
	accessMutex *syncutils.StarvingMutex
}

// NewLockableGormDB creates a new LockableGormDB instance.
func NewLockableGormDB(db *gorm.DB) *LockableGormDB {
	return &LockableGormDB{
		DB:          db,
		accessMutex: &syncutils.StarvingMutex{},
	}
}

// LockAccess locks access to the DB.
func (d *LockableGormDB) LockAccess() {
	d.accessMutex.Lock()
}

// UnlockAccess unlocks access to the DB.
func (d *LockableGormDB) UnlockAccess() {
	d.accessMutex.Unlock()
}

// CloseWithoutLocking closes the DB without locking access to it.
func (d *LockableGormDB) CloseWithoutLocking() error {
	db, err := d.DB.DB()
	if err != nil {
		return err
	}

	return db.Close()
}

// Close closes the DB.
func (d *LockableGormDB) Close() error {
	d.accessMutex.Lock()
	defer d.accessMutex.Unlock()

	return d.CloseWithoutLocking()
}

// ExecDBFunc executes a function with the DB as argument.
// The DB is locked during the execution of the function.
func (d *LockableGormDB) ExecDBFunc(f func(db *gorm.DB) error) error {
	d.accessMutex.RLock()
	defer d.accessMutex.RUnlock()

	return f(d.DB)
}
