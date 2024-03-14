package notarization

import (
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Notarization interface {
	// IsBootstrapped returns if notarization finished committing all pending slots up to the current acceptance time.
	IsBootstrapped() bool

	ForceCommit(slot iotago.SlotIndex) (*model.Commitment, error)
	ForceCommitUntil(commitUntilSlot iotago.SlotIndex) error

	AcceptedBlocksCount(index iotago.SlotIndex) int

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()

	module.Module
}
