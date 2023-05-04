package mempool

import "golang.org/x/xerrors"

var (
	ErrAttachmentNotFound       = xerrors.New("block not found")
	ErrTransactionNotFound      = xerrors.New("transaction not found")
	ErrTransactionExistsAlready = xerrors.New("transaction exists already")
)
