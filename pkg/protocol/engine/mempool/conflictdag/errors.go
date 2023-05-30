package conflictdag

import "golang.org/x/xerrors"

var (
	ErrAlreadyPartOfConflictSet = xerrors.New("conflict already part of ConflictSet")
	ErrEntityEvicted            = xerrors.New("tried to operate on evicted entity")
	ErrFatal                    = xerrors.New("fatal error")
)
