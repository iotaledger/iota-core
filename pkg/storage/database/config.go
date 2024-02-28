package database

import (
	"github.com/iotaledger/hive.go/db"
)

type Config struct {
	Engine       db.Engine
	Directory    string
	Version      byte
	PrefixHealth []byte
}

func (c Config) WithDirectory(directory string) Config {
	c.Directory = directory
	return c
}
