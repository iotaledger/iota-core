package storage

import (
	"gorm.io/gorm"
)

type (
	SQLDatabaseExecFunc func(func(*gorm.DB) error) error
)

func (s *Storage) TransactionRetainerDatabaseExecFunc() SQLDatabaseExecFunc {
	return s.txRetainerSQL.ExecDBFunc()
}
