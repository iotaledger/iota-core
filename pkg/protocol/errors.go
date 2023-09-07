package protocol

import "github.com/iotaledger/hive.go/ierrors"

var (
	ErrorCommitmentNotFound = ierrors.New("commitment not found")
	ErrorSlotEvicted        = ierrors.New("slot evicted")
)
