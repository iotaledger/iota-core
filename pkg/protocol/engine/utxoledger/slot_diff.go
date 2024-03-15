package utxoledger

import (
	"crypto/sha256"
	"sort"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/stream"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SlotDiff represents the generated and spent outputs by a slot's confirmation.
type SlotDiff struct {
	// The index of the slot.
	Slot iotago.SlotIndex
	// The outputs newly generated with this diff.
	Outputs Outputs
	// The outputs spent with this diff.
	Spents Spents
}

func slotDiffKeyForIndex(slot iotago.SlotIndex) []byte {
	byteBuffer := stream.NewByteBuffer(serializer.OneByte + iotago.SlotIndexLength)

	// There can't be any errors.
	_ = stream.Write(byteBuffer, StoreKeyPrefixSlotDiffs)
	_ = stream.Write(byteBuffer, slot)

	return lo.PanicOnErr(byteBuffer.Bytes())
}

func (sd *SlotDiff) KVStorableKey() []byte {
	return slotDiffKeyForIndex(sd.Slot)
}

func (sd *SlotDiff) KVStorableValue() []byte {
	byteBuffer := stream.NewByteBuffer()

	// There can't be any errors.
	_ = stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for _, output := range sd.sortedOutputs() {
			_ = stream.Write(byteBuffer, output.outputID)
		}

		return len(sd.Outputs), nil
	})

	_ = stream.WriteCollection(byteBuffer, serializer.SeriLengthPrefixTypeAsUint32, func() (elementsCount int, err error) {
		for _, spent := range sd.sortedSpents() {
			_ = stream.Write(byteBuffer, spent.output.outputID)
		}

		return len(sd.Spents), nil
	})

	return lo.PanicOnErr(byteBuffer.Bytes())
}

// note that this method relies on the data being available within other "tables".
func (sd *SlotDiff) kvStorableLoad(manager *Manager, key []byte, value []byte) error {
	var err error

	if sd.Slot, _, err = iotago.SlotIndexFromBytes(key[1:]); err != nil {
		return err
	}

	byteReader := stream.NewByteReader(value)

	outputsCount, err := stream.PeekSize(byteReader, serializer.SeriLengthPrefixTypeAsUint32)
	if err != nil {
		return ierrors.Wrap(err, "unable to peek outputs count")
	}

	outputs := make(Outputs, outputsCount)
	if err = stream.ReadCollection(byteReader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		outputID, err := stream.Read[iotago.OutputID](byteReader)
		if err != nil {
			return ierrors.Wrap(err, "unable to read outputID")
		}

		output, err := manager.ReadOutputByOutputIDWithoutLocking(outputID)
		if err != nil {
			return err
		}

		outputs[i] = output

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "unable to read slot diff outputs")
	}

	spentsCount, err := stream.PeekSize(byteReader, serializer.SeriLengthPrefixTypeAsUint32)
	if err != nil {
		return ierrors.Wrap(err, "unable to peek spents count")
	}

	spents := make(Spents, spentsCount)
	if err = stream.ReadCollection(byteReader, serializer.SeriLengthPrefixTypeAsUint32, func(i int) error {
		outputID, err := stream.Read[iotago.OutputID](byteReader)
		if err != nil {
			return ierrors.Wrap(err, "unable to read outputID")
		}

		spent, err := manager.ReadSpentForOutputIDWithoutLocking(outputID)
		if err != nil {
			return err
		}

		spents[i] = spent

		return nil
	}); err != nil {
		return ierrors.Wrapf(err, "unable to read slot diff spents")
	}

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

	if err := stream.WriteBytes(sdDiffHash, sd.KVStorableKey()); err != nil {
		return nil, ierrors.Wrap(err, "unable to serialize slot diff")
	}

	if err := stream.WriteBytes(sdDiffHash, sd.KVStorableValue()); err != nil {
		return nil, ierrors.Wrap(err, "unable to serialize slot diff")
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
