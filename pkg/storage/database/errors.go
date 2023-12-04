package database

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrEpochPruned       = ierrors.New("epoch pruned")
	ErrNoPruningNeeded   = ierrors.New("no pruning needed")
	ErrDatabaseFull      = ierrors.New("database full")
	ErrDatabaseShutdown  = ierrors.New("cannot open DBInstance that is shutdown")
	ErrDatabaseNotClosed = ierrors.New("cannot open DBInstance that is not closed")
)
