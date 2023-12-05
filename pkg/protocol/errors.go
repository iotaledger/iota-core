package protocol

import (
	"github.com/iotaledger/hive.go/ierrors"
)

var (
	// ErrorCommitmentNotFound is returned for requests for commitments that are not available yet.
	ErrorCommitmentNotFound = ierrors.New("commitment not found")

	// ErrorSlotEvicted is returned for requests for commitments that belong to evicted slots.
	ErrorSlotEvicted = ierrors.New("slot evicted")
)
