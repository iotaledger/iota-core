package conflictdag

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrExpected                 = ierrors.New("expected error")
	ErrAlreadyPartOfConflictSet = ierrors.New("conflict already part of ConflictSet")
	ErrEntityEvicted            = ierrors.New("tried to operate on evicted entity")
	ErrFatal                    = ierrors.New("fatal error")
)
