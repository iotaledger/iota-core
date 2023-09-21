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
	slotPrefixAttestations
	slotPrefixAccountDiffs
	slotPrefixPerformanceFactors
	slotPrefixUpgradeSignals
	slotPrefixRoots
	slotPrefixRetainer
)

func (p *Prunable) getKVStoreFromSlot(slot iotago.SlotIndex, prefix kvstore.Realm) (kvstore.KVStore, error) {
	epoch := p.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot)

	return p.prunableSlotStore.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), prefix))
}

func (p *Prunable) Blocks(slot iotago.SlotIndex) (*slotstore.Blocks, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixBlocks})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get blocks with slot %d", slot)
	}

	return slotstore.NewBlocks(slot, kv, p.apiProvider.APIForSlot(slot)), nil
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) (*slotstore.Store[iotago.BlockID, iotago.CommitmentID], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRootBlocks})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get root blocks with slot %d", slot)
	}

	return slotstore.NewStore(slot, kv,
		iotago.SlotIdentifier.Bytes,
		iotago.SlotIdentifierFromBytes,
		iotago.SlotIdentifier.Bytes, iotago.SlotIdentifierFromBytes,
	), nil
}

func (p *Prunable) Attestations(slot iotago.SlotIndex) (kvstore.KVStore, error) {
	return p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAttestations})
}

func (p *Prunable) AccountDiffs(slot iotago.SlotIndex) (*slotstore.AccountDiffs, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAccountDiffs})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get account diffs with slot %d", slot)
	}

	return slotstore.NewAccountDiffs(slot, kv, p.apiProvider.APIForSlot(slot)), nil
}

func (p *Prunable) ValidatorPerformances(slot iotago.SlotIndex) (*slotstore.Store[iotago.AccountID, *model.ValidatorPerformance], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixPerformanceFactors})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get performance factors with slot %d", slot)
	}

	apiForSlot := p.apiProvider.APIForSlot(slot)

	return slotstore.NewStore(slot, kv,
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		func(s *model.ValidatorPerformance) ([]byte, error) {
			return s.Bytes(apiForSlot)
		},
		model.ValidatorPerformanceFromBytes(apiForSlot),
	), nil
}

func (p *Prunable) UpgradeSignals(slot iotago.SlotIndex) (*slotstore.Store[account.SeatIndex, *model.SignaledBlock], error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixUpgradeSignals})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get upgrade signals with slot %d", slot)
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
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get roots with slot %d", slot)
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
		iotago.SlotIdentifierFromBytes,
		rootsBytes,
		rootsFromBytes,
	), nil
}

func (p *Prunable) Retainer(slot iotago.SlotIndex) (*slotstore.Retainer, error) {
	kv, err := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRetainer})
	if err != nil {
		return nil, ierrors.Wrapf(database.ErrEpochPruned, "could not get retainer with slot %d", slot)
	}

	return slotstore.NewRetainer(slot, kv), nil
}
