package mempool

import (
	"github.com/iotaledger/hive.go/ierrors"
)

var (
	ErrStateNotFound                    = ierrors.New("state not found")
	ErrInputSolidificationRequestFailed = ierrors.New("UTXO input solidification failed")
)
