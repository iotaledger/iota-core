package prunable

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	slotPrefixBlocks byte = iota
	slotPrefixRootBlocks
	slotPrefixMutations
	slotPrefixAttestations
	slotPrefixAccountDiffs
	slotPrefixPerformanceFactors
	slotPrefixUpgradeSignals
	slotPrefixRoots
	slotPrefixBlockMetadata
	epochPrefixCommitteeCandidates
)

func (p *Prunable) getKVStoreFromSlot(slot iotago.SlotIndex, prefix kvstore.Realm) (kvstore.KVStore, error) {
	epoch := p.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot)

	return p.prunableSlotStore.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), prefix))
}

func (p *Prunable) Blocks(slot iotago.SlotIndex) (*slotstore.Blocks, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixBlocks})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get blocks with slot %d", slot)
	}

	return slotstore.NewBlocks(slot, kv, p.apiProvider.APIForSlot(slot)), nil
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRootBlocks})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get root blocks with slot %d", slot)
	}

	return slotstore.NewStore(slot, kv,
		iotago.BlockID.Bytes,
		iotago.BlockIDFromBytes,
		iotago.CommitmentID.Bytes,
		iotago.CommitmentIDFromBytes,
	), nil
}

func (p *Prunable) CommitteeCandidates(epoch iotago.EpochIndex) (*kvstore.TypedStore[iotago.AccountID, iotago.SlotIndex], error) {
	// Use the first slot of an epoch to avoid random clashes with other keys.
	// Candidates belong to an epoch, but we store them here so that they're pruned more quickly and easily without unnecessary key iteration.
	kv, err := p.prunableSlotStore.Get(epoch, byteutils.ConcatBytes(p.apiProvider.APIForEpoch(epoch).TimeProvider().EpochStart(epoch).MustBytes(), kvstore.Realm{epochPrefixCommitteeCandidates}))
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get committee candidates with epoch %d", epoch)
	}

	return kvstore.NewTypedStore(kv,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
		iotago.SlotIndex.Bytes,
		iotago.SlotIndexFromBytes,
	), nil
}

func (p *Prunable) Mutations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	return p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixMutations})
}

func (p *Prunable) Attestations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	return p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAttestations})
}

func (p *Prunable) AccountDiffs(slot iotago.SlotIndex) (*slotstore.AccountDiffs, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAccountDiffs})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get account diffs with slot %d", slot)
	}

	return slotstore.NewAccountDiffs(slot, kv, p.apiProvider.APIForSlot(slot)), nil
}

func (p *Prunable) ValidatorPerformances(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixPerformanceFactors})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get performance factors with slot %d", slot)
	}

	return slotstore.NewStore(slot, kv,
		iotago.AccountID.Bytes,
		iotago.AccountIDFromBytes,
		(*model.ValidatorPerformance).Bytes,
		model.ValidatorPerformanceFromBytes,
	), nil
}

func (p *Prunable) UpgradeSignals(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixUpgradeSignals})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get upgrade signals with slot %d", slot)
	}

	apiForSlot := p.apiProvider.APIForSlot(slot)

	return slotstore.NewStore(slot, kv,
		account.SeatIndex.Bytes,
		account.SeatIndexFromBytes,
		func(s *model.SignaledBlock) ([]byte, error) {
			return s.Bytes(apiForSlot)
		},
		model.SignaledBlockFromBytesFunc(apiForSlot),
	), nil
}

func (p *Prunable) Roots(slot iotago.SlotIndex) (*slotstore.Store[iotago.CommitmentID, *iotago.Roots], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRoots})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get roots with slot %d", slot)
	}

	apiForSlot := p.apiProvider.APIForSlot(slot)
	rootsBytes := func(roots *iotago.Roots) ([]byte, error) {
		return apiForSlot.Encode(roots)
	}
	rootsFromBytes := func(b []byte) (*iotago.Roots, int, error) {
		var roots iotago.Roots
		readBytes, err := apiForSlot.Decode(b, &roots)
		if err != nil {
			return nil, 0, err
		}

		return &roots, readBytes, nil
	}

	return slotstore.NewStore(slot, kv,
		iotago.CommitmentID.Bytes,
		iotago.CommitmentIDFromBytes,
		rootsBytes,
		rootsFromBytes,
	), nil
}

func (p *Prunable) BlockMetadata(slot iotago.SlotIndex) (*slotstore.BlockMetadataStore, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixBlockMetadata})
	if err != nil {
		return nil, ierrors.WithMessagef(database.ErrEpochPruned, "could not get block metadata store with slot %d", slot)
	}

	return slotstore.NewBlockMetadataStore(slot, kv), nil
}
