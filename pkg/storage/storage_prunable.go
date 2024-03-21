package storage

import (
	"io"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Storage) RewardsForEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
	return s.prunable.RewardsForEpoch(epoch)
}

func (s *Storage) Rewards() *epochstore.EpochKVStore {
	return s.prunable.Rewards()
}

func (s *Storage) PoolStats() epochstore.Store[*model.PoolsStats] {
	return s.prunable.PoolStats()
}

func (s *Storage) DecidedUpgradeSignals() epochstore.Store[model.VersionAndHash] {
	return s.prunable.DecidedUpgradeSignals()
}

func (s *Storage) Committee() epochstore.Store[*account.SeatedAccounts] {
	return s.prunable.Committee()
}

func (s *Storage) CommitteeCandidates(epoch iotago.EpochIndex) (*kvstore.TypedStore[iotago.AccountID, iotago.SlotIndex], error) {
	return s.prunable.CommitteeCandidates(epoch)
}

func (s *Storage) Blocks(slot iotago.SlotIndex) (*slotstore.Blocks, error) {
	s.lastAccessedBlocks.Compute(func(lastAccessedBlocks iotago.SlotIndex) iotago.SlotIndex {
		return max(lastAccessedBlocks, slot)
	})

	return s.prunable.Blocks(slot)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (s *Storage) Reset() {
	if err := s.Rollback(s.Settings().LatestCommitment().Slot()); err != nil {
		s.errorHandler(ierrors.Wrap(err, "failed to reset prunable storage"))
	}
}

func (s *Storage) RootBlocks(slot iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing root blocks")
	}

	return s.prunable.RootBlocks(slot)
}

func (s *Storage) GenesisRootBlockID() iotago.BlockID {
	return s.Settings().APIProvider().CommittedAPI().ProtocolParameters().GenesisBlockID()
}

func (s *Storage) Mutations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing mutations")
	}

	return s.prunable.Mutations(slot)
}

func (s *Storage) Attestations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing attestations")
	}

	return s.prunable.Attestations(slot)
}

func (s *Storage) AccountDiffs(slot iotago.SlotIndex) (*slotstore.AccountDiffs, error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing account diffs")
	}

	return s.prunable.AccountDiffs(slot)
}

func (s *Storage) ValidatorPerformances(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing validator performances")
	}

	return s.prunable.ValidatorPerformances(slot)
}

func (s *Storage) UpgradeSignals(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing upgrade signals")
	}

	return s.prunable.UpgradeSignals(slot)
}

func (s *Storage) Roots(slot iotago.SlotIndex) (*slotstore.Store[iotago.CommitmentID, *iotago.Roots], error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing roots")
	}

	return s.prunable.Roots(slot)
}

func (s *Storage) ExportRoots(writer io.WriteSeeker, targetCommitment *iotago.Commitment) error {
	slotIndex := targetCommitment.Slot

	if slotIndex <= s.Settings().APIProvider().CommittedAPI().ProtocolParameters().GenesisSlot() {
		return nil
	}

	commitmentID, err := targetCommitment.ID()

	if err != nil {
		return ierrors.Wrap(err, "can not retrieve commitment id")
	}
	// Load root storage from prunable storage
	rootsStorage, errRoots := s.Roots(slotIndex)

	if errRoots != nil {
		return ierrors.Wrap(err, "failed to load roots storage")
	}

	roots, exists, errLoad := rootsStorage.Load(commitmentID)
	if errLoad != nil {
		return ierrors.Wrap(err, "failed to load roots from prunable storage")
	} else if !exists {
		return ierrors.Wrap(err, "roots not found")
	}

	if errWrite := stream.Write(writer, roots.AccountRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write account root bytes")
	}
	if errWrite := stream.Write(writer, roots.AttestationsRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write attestation root bytes")
	}
	if errWrite := stream.Write(writer, roots.CommitteeRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write committee root bytes")
	}
	if errWrite := stream.Write(writer, roots.ProtocolParametersHash); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write protocol parameters hash root bytes")
	}
	if errWrite := stream.Write(writer, roots.RewardsRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write rewards root bytes")
	}
	if errWrite := stream.Write(writer, roots.StateMutationRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write state mutation root bytes")
	}
	if errWrite := stream.Write(writer, roots.StateRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write state root bytes")
	}
	if errWrite := stream.Write(writer, roots.TangleRoot); errWrite != nil {
		return ierrors.Wrapf(err, "failed to write tangle root bytes")
	}

	return err
}

func (s *Storage) ImportRoots(reader io.ReadSeeker, targetCommitment *model.Commitment) error {
	slotIndex := targetCommitment.Commitment().Slot

	if slotIndex <= s.Settings().APIProvider().CommittedAPI().ProtocolParameters().GenesisSlot() {
		return nil
	}

	accountRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve account root")
	}

	attestationRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve attestation root")
	}

	committeeRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve committee root")
	}

	protocolParametersHash, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve protocol parameters hash")
	}

	rewardsRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve rewards root")
	}

	stateMutationRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve state mutation root")
	}

	stateRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve state root")
	}

	tangleRoot, err := stream.Read[iotago.Identifier](reader)
	if err != nil {
		return ierrors.Wrap(err, "can not retrieve tangle root")
	}

	commitmentID, err := targetCommitment.Commitment().ID()

	if err != nil {
		return ierrors.Wrap(err, "can not retrieve commitment id")
	}
	// Load root storage from prunable storage
	rootsStorage, errRoots := s.Roots(slotIndex)

	if errRoots != nil {
		return ierrors.Wrap(err, "failed to load roots storage")
	}

	roots := iotago.NewRoots(
		tangleRoot,
		stateMutationRoot,
		attestationRoot,
		stateRoot,
		accountRoot,
		committeeRoot,
		rewardsRoot,
		protocolParametersHash,
	)

	errStore := rootsStorage.Store(commitmentID, roots)
	if errStore != nil {
		return ierrors.Wrap(err, "unable to store roots in storage")
	}

	return nil
}

func (s *Storage) BlockMetadata(slot iotago.SlotIndex) (*slotstore.BlockMetadataStore, error) {
	if err := s.permanent.Settings().AdvanceLatestStoredSlot(slot); err != nil {
		return nil, ierrors.Wrap(err, "failed to advance latest stored slot when accessing block metadata")
	}

	return s.prunable.BlockMetadata(slot)
}

func (s *Storage) pruningRange(targetSlot iotago.SlotIndex) (targetEpoch iotago.EpochIndex, startSlot iotago.SlotIndex, endSlot iotago.SlotIndex) {
	timeProvider := s.Settings().APIProvider().APIForSlot(targetSlot).TimeProvider()

	targetEpoch = timeProvider.EpochFromSlot(targetSlot)

	startSlot = targetSlot + 1
	endSlot = s.Settings().LatestStoredSlot()

	// If startSlot is in the next epoch, there's no need to prune a range of slots as the next epoch is going to be pruned on epoch-level anyway.
	if timeProvider.EpochFromSlot(startSlot) > targetEpoch {
		endSlot = 0
	}

	return targetEpoch, startSlot, endSlot
}
