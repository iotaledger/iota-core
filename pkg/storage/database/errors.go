package database

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrEpochPruned      = ierrors.New("epoch pruned")
	ErrNotEnoughHistory = ierrors.New("not enough history")
	ErrNoPruningNeeded  = ierrors.New("no pruning needed")
	ErrPruningAborted   = ierrors.New("pruning was aborted")
)
