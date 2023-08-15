package prunable

import (
	"bytes"
	"encoding/binary"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/byteutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
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

func (p *Prunable) getKVStoreFromSlot(slot iotago.SlotIndex, prefix kvstore.Realm) kvstore.KVStore {
	epoch := p.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot)
	if p.IsTooOld(epoch) {
		return nil
	}

	return p.prunableSlotStore.Get(epoch, byteutils.ConcatBytes(slot.MustBytes(), prefix))
}

func (p *Prunable) Blocks(slot iotago.SlotIndex) *slotstore.Blocks {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixBlocks})
	if kv == nil {
		return nil
	}

	return slotstore.NewBlocks(slot, kv, p.apiProvider.APIForSlot(slot))
}

func (p *Prunable) RootBlocks(slot iotago.SlotIndex) *slotstore.Store[iotago.BlockID, iotago.CommitmentID] {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRootBlocks})
	if kv == nil {
		return nil
	}

	return slotstore.NewStore(slot, kv,
		iotago.SlotIdentifier.Bytes,
		iotago.SlotIdentifierFromBytes,
		iotago.SlotIdentifier.Bytes,
		iotago.SlotIdentifierFromBytes,
	)
}

func (p *Prunable) Attestations(slot iotago.SlotIndex) kvstore.KVStore {
	return p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAttestations})
}

func (p *Prunable) AccountDiffs(slot iotago.SlotIndex) *slotstore.AccountDiffs {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixAccountDiffs})
	if kv == nil {
		return nil
	}

	return slotstore.NewAccountDiffs(slot, kv, p.apiProvider.APIForSlot(slot))
}

func (p *Prunable) PerformanceFactors(slot iotago.SlotIndex) *slotstore.Store[iotago.AccountID, uint64] {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixPerformanceFactors})
	if kv == nil {
		return nil
	}

	uint64Bytes := func(value uint64) ([]byte, error) {
		buf := bytes.NewBuffer(make([]byte, 0, serializer.UInt64ByteSize))
		if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
			return nil, err
		}

		return buf.Bytes(), nil
	}

	uint64FromBytes := func(b []byte) (uint64, int, error) {
		buf := bytes.NewBuffer(b)
		var value uint64
		if err := binary.Read(buf, binary.LittleEndian, &value); err != nil {
			return 0, 0, err
		}

		return value, serializer.UInt64ByteSize, nil
	}

	return slotstore.NewStore(slot, kv,
		iotago.AccountID.Bytes,
		iotago.IdentifierFromBytes,
		uint64Bytes,
		uint64FromBytes,
	)
}
func (p *Prunable) UpgradeSignals(slot iotago.SlotIndex) *slotstore.Store[account.SeatIndex, *model.SignaledBlock] {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixUpgradeSignals})
	if kv == nil {
		return nil
	}

	apiForSlot := p.apiProvider.APIForSlot(slot)

	return slotstore.NewStore(slot, kv,
		account.SeatIndex.Bytes,
		account.SeatIndexFromBytes,
		func(s *model.SignaledBlock) ([]byte, error) {
			return s.Bytes(apiForSlot)
		},
		model.SignaledBlockFromBytesFunc(apiForSlot),
	)
}

func (p *Prunable) Roots(slot iotago.SlotIndex) *slotstore.Store[iotago.CommitmentID, *iotago.Roots] {
	kv := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRoots})
	if kv == nil {
		return nil
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
	)
}

func (p *Prunable) Retainer(slot iotago.SlotIndex) *slotstore.Retainer {
	store := p.getKVStoreFromSlot(slot, kvstore.Realm{slotPrefixRetainer})
	if store == nil {
		return nil
	}

	return slotstore.NewRetainer(slot, store)
}
