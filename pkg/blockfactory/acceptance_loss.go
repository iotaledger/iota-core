package blockfactory

import (
	"time"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (i *BlockIssuer) reviveChain(issuingTime time.Time) (*iotago.Commitment, iotago.BlockID, error) {
	lastCommittedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Slot()
	apiForSlot := i.protocol.APIForSlot(lastCommittedSlot)

	// Get a parent from the last committed slot.
	store, err := i.protocol.MainEngineInstance().Storage.Blocks(lastCommittedSlot)
	if err != nil {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to get blocks for slot %d", lastCommittedSlot)
	}

	expectedErr := ierrors.New("stop iteration")
	parentBlockID := iotago.EmptyBlockID()
	if err = store.StreamKeys(func(blockID iotago.BlockID) error {
		parentBlockID = blockID
		return expectedErr
	}); err != nil && !ierrors.Is(err, expectedErr) {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to stream block IDs for slot %d", lastCommittedSlot)
	}

	// Get last issued block and start issuing after that subslot.
	// TODO: also make sure to not issue on two distinct chains within the same slot.
	lastIssuedBlockValidationBlock := i.protocol.MainEngineInstance().Storage.Settings().LatestIssuedValidationBlock()
	if lastIssuedBlockValidationBlock != nil {

	}
	issuingSlot := apiForSlot.TimeProvider().SlotFromTime(issuingTime)

	// Force commitments until minCommittableAge relative to the block's issuing time. We basically "pretend" that
	// this block was already accepted at the time of issuing so that we have a commitment to reference.
	if issuingSlot < apiForSlot.ProtocolParameters().MinCommittableAge() { // Should never happen as we're beyond maxCommittableAge which is > minCommittableAge.
		return nil, iotago.EmptyBlockID(), ierrors.Errorf("issuing slot %d is smaller than min committable age %d", issuingSlot, apiForSlot.ProtocolParameters().MinCommittableAge())
	}
	commitUntilSlot := issuingSlot - apiForSlot.ProtocolParameters().MinCommittableAge()

	if err = i.protocol.MainEngineInstance().Notarization.ForceCommitUntil(commitUntilSlot); err != nil {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to force commit until slot %d", commitUntilSlot)
	}

	commitment, err := i.protocol.MainEngineInstance().Storage.Commitments().Load(commitUntilSlot)
	if err != nil {
		return nil, iotago.EmptyBlockID(), ierrors.Wrapf(err, "failed to load commitment for slot %d", commitUntilSlot)
	}

	return commitment.Commitment(), parentBlockID, nil
}
