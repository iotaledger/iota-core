package prunablesql

import (
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/runtime/syncutils"
)

func NewLockableGormDB(db *gorm.DB) *LockableGormDB {
	return &LockableGormDB{
		DB:          db,
		accessMutex: &syncutils.StarvingMutex{},
	}
}

type LockableGormDB struct {
	DB          *gorm.DB
	accessMutex *syncutils.StarvingMutex
}

func (d *LockableGormDB) LockAccess() {
	d.accessMutex.Lock()
}

func (d *LockableGormDB) UnlockAccess() {
	d.accessMutex.Unlock()
}

func (d *LockableGormDB) CloseWithoutLocking() error {
	db, err := d.DB.DB()
	if err != nil {
		return err
	}

	return db.Close()
}

func (d *LockableGormDB) Close() error {
	d.accessMutex.Lock()
	defer d.accessMutex.Unlock()

	return d.CloseWithoutLocking()
}

func (d *LockableGormDB) ExecDBFunc(f func(db *gorm.DB) error) error {
	d.accessMutex.RLock()
	defer d.accessMutex.RUnlock()

	return f(d.DB)
}
