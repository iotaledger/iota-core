package ledger

import "golang.org/x/xerrors"

var (
	ErrStateNotFound = xerrors.New("state not found")
)
