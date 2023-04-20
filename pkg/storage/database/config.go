package database

import hivedb "github.com/iotaledger/hive.go/kvstore/database"

type Config struct {
	Engine    hivedb.Engine
	Directory string

	Version      byte
	PrefixHealth []byte
}

func (c Config) WithDirectory(directory string) Config {
	c.Directory = directory
	return c
}
