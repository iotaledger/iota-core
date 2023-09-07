package database

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrEpochPruned     = ierrors.New("epoch pruned")
	ErrNoPruningNeeded = ierrors.New("no pruning needed")
	ErrDatabaseFull    = ierrors.New("database full")
)
