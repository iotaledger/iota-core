package utxoledger

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotDiff represents the generated and spent outputs by a slot's confirmation.
type SlotDiff struct {
	// The index of the slot.
	Index iotago.SlotIndex
	// The outputs newly generated with this diff.
	Outputs Outputs
	// The outputs spent with this diff.
	Spents Spents
}

func slotDiffKeyForIndex(index iotago.SlotIndex) []byte {
	m := marshalutil.New(9)
	m.WriteByte(StoreKeyPrefixSlotDiffs)
	m.WriteBytes(index.MustBytes())

	return m.Bytes()
}

func (sd *SlotDiff) KVStorableKey() []byte {
	return slotDiffKeyForIndex(sd.Index)
}

func (sd *SlotDiff) KVStorableValue() []byte {
	m := marshalutil.New(9)

	m.WriteUint32(uint32(len(sd.Outputs)))
	for _, output := range sd.sortedOutputs() {
		m.WriteBytes(output.outputID[:])
	}

	m.WriteUint32(uint32(len(sd.Spents)))
	for _, spent := range sd.sortedSpents() {
		m.WriteBytes(spent.output.outputID[:])
	}

	return m.Bytes()
}

// note that this method relies on the data being available within other "tables".
func (sd *SlotDiff) kvStorableLoad(manager *Manager, key []byte, value []byte) error {
	index, _, err := iotago.SlotIndexFromBytes(key[1:])
	if err != nil {
		return err
	}

	marshalUtil := marshalutil.New(value)

	outputCount, err := marshalUtil.ReadUint32()
	if err != nil {
		return err
	}

	outputs := make(Outputs, int(outputCount))
	for i := 0; i < int(outputCount); i++ {
		var outputID iotago.OutputID
		if outputID, err = ParseOutputID(marshalUtil); err != nil {
			return err
		}

		output, err := manager.ReadOutputByOutputIDWithoutLocking(outputID)
		if err != nil {
			return err
		}

		outputs[i] = output
	}

	spentCount, err := marshalUtil.ReadUint32()
	if err != nil {
		return err
	}

	spents := make(Spents, spentCount)
	for i := 0; i < int(spentCount); i++ {
		var outputID iotago.OutputID
		if outputID, err = ParseOutputID(marshalUtil); err != nil {
			return err
		}

		spent, err := manager.ReadSpentForOutputIDWithoutLocking(outputID)
		if err != nil {
			return err
		}

		spents[i] = spent
	}

	sd.Index = index
	sd.Outputs = outputs
	sd.Spents = spents

	return nil
}

func (sd *SlotDiff) sortedOutputs() LexicalOrderedOutputs {
	// do not sort in place
	sortedOutputs := make(LexicalOrderedOutputs, len(sd.Outputs))
	copy(sortedOutputs, sd.Outputs)
	sort.Sort(sortedOutputs)

	return sortedOutputs
}

func (sd *SlotDiff) sortedSpents() LexicalOrderedSpents {
	// do not sort in place
	sortedSpents := make(LexicalOrderedSpents, len(sd.Spents))
	copy(sortedSpents, sd.Spents)
	sort.Sort(sortedSpents)

	return sortedSpents
}

// SHA256Sum computes the sha256 of the slot diff byte representation.
func (sd *SlotDiff) SHA256Sum() ([]byte, error) {
	sdDiffHash := sha256.New()

	if err := binary.Write(sdDiffHash, binary.LittleEndian, sd.KVStorableKey()); err != nil {
		return nil, ierrors.Errorf("unable to serialize slot diff: %w", err)
	}

	if err := binary.Write(sdDiffHash, binary.LittleEndian, sd.KVStorableValue()); err != nil {
		return nil, ierrors.Errorf("unable to serialize slot diff: %w", err)
	}

	// calculate sha256 hash
	return sdDiffHash.Sum(nil), nil
}

// DB helper functions.

func storeDiff(diff *SlotDiff, mutations kvstore.BatchedMutations) error {
	return mutations.Set(diff.KVStorableKey(), diff.KVStorableValue())
}

func deleteDiff(index iotago.SlotIndex, mutations kvstore.BatchedMutations) error {
	return mutations.Delete(slotDiffKeyForIndex(index))
}

// Manager functions.

func (m *Manager) SlotDiffWithoutLocking(index iotago.SlotIndex) (*SlotDiff, error) {
	key := slotDiffKeyForIndex(index)

	value, err := m.store.Get(key)
	if err != nil {
		return nil, err
	}

	diff := &SlotDiff{}
	if err := diff.kvStorableLoad(m, key, value); err != nil {
		return nil, err
	}

	return diff, nil
}

func (m *Manager) SlotDiff(index iotago.SlotIndex) (*SlotDiff, error) {
	m.ReadLockLedger()
	defer m.ReadUnlockLedger()

	return m.SlotDiffWithoutLocking(index)
}

// code guards.
var _ kvStorable = &SlotDiff{}
