package database

import hivedb "github.com/iotaledger/hive.go/kvstore/database"

type Config struct {
	Engine    hivedb.Engine
	Directory string

	Version      byte
	PrefixHealth []byte
}
