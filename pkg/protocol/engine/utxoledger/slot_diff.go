package utxoledger

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotDiff represents the generated and spent outputs by a slot's confirmation.
type SlotDiff struct {
	// The index of the milestone.
	Index iotago.SlotIndex
	// The outputs newly generated with this diff.
	Outputs Outputs
	// The outputs spent with this diff.
	Spents Spents
}

func slotDiffKeyForIndex(index iotago.SlotIndex) []byte {
	m := marshalutil.New(9)
	m.WriteByte(StoreKeyPrefixSlotDiffs)
	m.WriteBytes(index.Bytes())

	return m.Bytes()
}

func (ms *SlotDiff) KVStorableKey() []byte {
	return slotDiffKeyForIndex(ms.Index)
}

func (ms *SlotDiff) KVStorableValue() []byte {
	m := marshalutil.New(9)

	m.WriteUint32(uint32(len(ms.Outputs)))
	for _, output := range ms.sortedOutputs() {
		m.WriteBytes(output.outputID[:])
	}

	m.WriteUint32(uint32(len(ms.Spents)))
	for _, spent := range ms.sortedSpents() {
		m.WriteBytes(spent.output.outputID[:])
	}

	return m.Bytes()
}

// note that this method relies on the data being available within other "tables".
func (ms *SlotDiff) kvStorableLoad(manager *Manager, key []byte, value []byte) error {
	index, err := iotago.SlotIndexFromBytes(key[1:])
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

	ms.Index = index
	ms.Outputs = outputs
	ms.Spents = spents

	return nil
}

func (ms *SlotDiff) sortedOutputs() LexicalOrderedOutputs {
	// do not sort in place
	sortedOutputs := make(LexicalOrderedOutputs, len(ms.Outputs))
	copy(sortedOutputs, ms.Outputs)
	sort.Sort(sortedOutputs)

	return sortedOutputs
}

func (ms *SlotDiff) sortedSpents() LexicalOrderedSpents {
	// do not sort in place
	sortedSpents := make(LexicalOrderedSpents, len(ms.Spents))
	copy(sortedSpents, ms.Spents)
	sort.Sort(sortedSpents)

	return sortedSpents
}

// SHA256Sum computes the sha256 of the milestone diff byte representation.
func (ms *SlotDiff) SHA256Sum() ([]byte, error) {
	msDiffHash := sha256.New()

	if err := binary.Write(msDiffHash, binary.LittleEndian, ms.KVStorableKey()); err != nil {
		return nil, fmt.Errorf("unable to serialize milestone diff: %w", err)
	}

	if err := binary.Write(msDiffHash, binary.LittleEndian, ms.KVStorableValue()); err != nil {
		return nil, fmt.Errorf("unable to serialize milestone diff: %w", err)
	}

	// calculate sha256 hash
	return msDiffHash.Sum(nil), nil
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
