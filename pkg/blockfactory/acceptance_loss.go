package blockfactory

import (
	"github.com/iotaledger/iota-core/pkg/model"
)

func (i *BlockIssuer) reviveChain(block *model.Block) error {
	lastCommittedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Index()
	apiForSlot := i.protocol.APIForSlot(lastCommittedSlot)

	// Get a parent from the last committed slot.

	// Get last issued block and start issuing after that subslot.
	// TODO: also make sure to not issue on two distinct chains within the same slot.
	lastIssuedBlockValidationBlock := i.protocol.MainEngineInstance().Storage.Settings().LatestIssuedValidationBlock()
	if lastIssuedBlockValidationBlock != nil {

	}

	// Force commitments until minCommittableAge relative to the block's issuing time. We basically "pretend" that
	// this block was already accepted at the time of issuing so that we have a commitment to reference.

	return nil
}
