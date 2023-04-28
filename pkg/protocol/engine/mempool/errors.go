package mempool

import "golang.org/x/xerrors"

var (
	ErrTransactionNotFound = xerrors.New("transaction not found")
)
