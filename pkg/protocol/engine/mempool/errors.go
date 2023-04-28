package mempool

import "golang.org/x/xerrors"

var (
	ErrTransactionNotFound      = xerrors.New("transaction not found")
	ErrTransactionExistsAlready = xerrors.New("transaction exists already")
)
