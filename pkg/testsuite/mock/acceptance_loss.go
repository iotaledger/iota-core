package blockfactory

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (i *BlockIssuer) reviveChain(issuingTime time.Time) (*iotago.Commitment, iotago.BlockID, error) {
	lastCommittedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot()
	apiForSlot := i.protocol.APIForSlot(lastCommittedSlot)

	// Get a rootblock as recent as possible for the parent.
	parentBlockID := iotago.EmptyBlockID()
	for rootBlock := range i.protocol.MainEngineInstance().EvictionState.ActiveRootBlocks() {
		if rootBlock.Slot() > parentBlockID.Slot() {
			parentBlockID = rootBlock
		}

		// Exit the loop if we found a rootblock in the last committed slot (which is the highest we can get).
		if parentBlockID.Slot() == lastCommittedSlot {
			break
		}
	}

	issuingSlot := apiForSlot.TimeProvider().SlotFromTime(issuingTime)

	// Force commitments until minCommittableAge relative to the block's issuing time. We basically "pretend" that
	// this block was already accepted at the time of issuing so that we have a commitment to reference.
	if issuingSlot < apiForSlot.ProtocolParameters().MinCommittableAge() { // Should never happen as we're beyond maxCommittableAge which is > minCommittableAge.
		return nil, iotago.EmptyBlockID(), ierrors.Errorf("issuing slot %d is smaller than min committable age %d", issuingSlot, apiForSlot.ProtocolParameters().MinCommittableAge())
	}
	commitUntilSlot := issuingSlot - apiForSlot.ProtocolParameters().MinCommittableAge()

	if err := i.protocol.MainEngineInstance().Notarization.ForceCommitUntil(commitUntilSlot); err != nil {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to force commit until slot %d", commitUntilSlot)
	}

	commitment, err := i.protocol.MainEngineInstance().Storage.Commitments().Load(commitUntilSlot)
	if err != nil {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to commit until slot %d to revive chain", commitUntilSlot)
	}

	return commitment.Commitment(), parentBlockID, nil
}
