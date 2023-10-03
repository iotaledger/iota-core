package utxoledger

import (
	"bytes"
	"sync"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/serializer/v2"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// LexicalOrderedOutputs are outputs ordered in lexical order by their outputID.
type LexicalOrderedOutputs []*Output

func (l LexicalOrderedOutputs) Len() int {
	return len(l)
}

func (l LexicalOrderedOutputs) Less(i, j int) bool {
	return bytes.Compare(l[i].outputID[:], l[j].outputID[:]) < 0
}

func (l LexicalOrderedOutputs) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

type Output struct {
	apiProvider iotago.APIProvider

	outputID   iotago.OutputID
	blockID    iotago.BlockID
	slotBooked iotago.SlotIndex

	encodedOutput []byte
	outputOnce    sync.Once
	output        iotago.Output
}

func (o *Output) StateID() iotago.Identifier {
	return iotago.IdentifierFromData(lo.PanicOnErr(o.outputID.Bytes()))
}

func (o *Output) Type() iotago.StateType {
	return iotago.InputUTXO
}

func (o *Output) IsReadOnly() bool {
	return false
}

func (o *Output) OutputID() iotago.OutputID {
	return o.outputID
}

func (o *Output) MapKey() string {
	return string(o.outputID[:])
}

func (o *Output) BlockID() iotago.BlockID {
	return o.blockID
}

func (o *Output) SlotBooked() iotago.SlotIndex {
	return o.slotBooked
}

func (o *Output) SlotCreated() iotago.SlotIndex {
	return o.outputID.CreationSlot()
}

func (o *Output) OutputType() iotago.OutputType {
	return o.Output().Type()
}

func (o *Output) Output() iotago.Output {
	o.outputOnce.Do(func() {
		if o.output == nil {
			var decoded iotago.TxEssenceOutput
			if _, err := o.apiProvider.APIForSlot(o.blockID.Slot()).Decode(o.encodedOutput, &decoded); err != nil {
				panic(err)
			}
			o.output = decoded
		}
	})

	return o.output
}

func (o *Output) Bytes() []byte {
	return o.encodedOutput
}

func (o *Output) BaseTokenAmount() iotago.BaseToken {
	return o.Output().BaseTokenAmount()
}

func (o *Output) StoredMana() iotago.Mana {
	return o.Output().StoredMana()
}

type Outputs []*Output

func (o Outputs) ToOutputSet() iotago.OutputSet {
	outputSet := make(iotago.OutputSet)
	for _, output := range o {
		outputSet[output.outputID] = output.Output()
	}

	return outputSet
}

func CreateOutput(apiProvider iotago.APIProvider, outputID iotago.OutputID, blockID iotago.BlockID, slotBooked iotago.SlotIndex, output iotago.Output, outputBytes ...[]byte) *Output {
	var encodedOutput []byte
	if len(outputBytes) == 0 {
		var err error
		encodedOutput, err = apiProvider.APIForSlot(blockID.Slot()).Encode(output)
		if err != nil {
			panic(err)
		}
	} else {
		encodedOutput = outputBytes[0]
	}

	o := &Output{
		apiProvider:   apiProvider,
		outputID:      outputID,
		blockID:       blockID,
		slotBooked:    slotBooked,
		encodedOutput: encodedOutput,
	}

	o.outputOnce.Do(func() {
		o.output = output
	})

	return o
}

func NewOutput(apiProvider iotago.APIProvider, blockID iotago.BlockID, slotBooked iotago.SlotIndex, transaction *iotago.Transaction, index uint16) (*Output, error) {
	txID, err := transaction.ID()
	if err != nil {
		return nil, err
	}

	var output iotago.Output
	if len(transaction.Outputs) <= int(index) {
		return nil, ierrors.New("output not found")
	}
	output = transaction.Outputs[int(index)]
	outputID := iotago.OutputIDFromTransactionIDAndIndex(txID, index)

	return CreateOutput(apiProvider, outputID, blockID, slotBooked, output), nil
}

// - kvStorable

func outputStorageKeyForOutputID(outputID iotago.OutputID) []byte {
	ms := marshalutil.New(iotago.OutputIDLength + 1)
	ms.WriteByte(StoreKeyPrefixOutput) // 1 byte
	ms.WriteBytes(outputID[:])         // iotago.OutputIDLength bytes

	return ms.Bytes()
}

func (o *Output) KVStorableKey() (key []byte) {
	return outputStorageKeyForOutputID(o.outputID)
}

func (o *Output) KVStorableValue() (value []byte) {
	ms := marshalutil.New()
	ms.WriteBytes(o.blockID[:])             // SlotIdentifierLength bytes
	ms.WriteBytes(o.slotBooked.MustBytes()) // 4 bytes
	ms.WriteBytes(o.encodedOutput)

	return ms.Bytes()
}

func (o *Output) kvStorableLoad(_ *Manager, key []byte, value []byte) error {
	// Parse key
	keyUtil := marshalutil.New(key)

	// Read prefix output
	_, err := keyUtil.ReadByte()
	if err != nil {
		return err
	}

	// Read OutputID
	if o.outputID, err = ParseOutputID(keyUtil); err != nil {
		return err
	}

	// Parse value
	valueUtil := marshalutil.New(value)

	// Read BlockID
	if o.blockID, err = ParseBlockID(valueUtil); err != nil {
		return err
	}

	// Read Slot
	o.slotBooked, err = parseSlotIndex(valueUtil)
	if err != nil {
		return err
	}

	o.encodedOutput = valueUtil.ReadRemainingBytes()

	return nil
}

// - Helper

func storeOutput(output *Output, mutations kvstore.BatchedMutations) error {
	return mutations.Set(output.KVStorableKey(), output.KVStorableValue())
}

func deleteOutput(output *Output, mutations kvstore.BatchedMutations) error {
	return mutations.Delete(output.KVStorableKey())
}

// - Manager

func (m *Manager) ReadOutputByOutputIDWithoutLocking(outputID iotago.OutputID) (*Output, error) {
	key := outputStorageKeyForOutputID(outputID)
	value, err := m.store.Get(key)
	if err != nil {
		return nil, err
	}

	output := &Output{
		apiProvider: m.apiProvider,
	}
	if err := output.kvStorableLoad(m, key, value); err != nil {
		return nil, err
	}

	return output, nil
}

func (m *Manager) ReadRawOutputBytesByOutputIDWithoutLocking(outputID iotago.OutputID) ([]byte, error) {
	key := outputStorageKeyForOutputID(outputID)
	value, err := m.store.Get(key)
	if err != nil {
		return nil, err
	}

	// blockID + slotIndex + timestampCreated
	offset := iotago.BlockIDLength + serializer.UInt64ByteSize + serializer.UInt64ByteSize
	if len(value) <= offset {
		return nil, ierrors.New("invalid UTXO output length")
	}

	return value[offset:], nil
}

func (m *Manager) ReadOutputByOutputID(outputID iotago.OutputID) (*Output, error) {
	m.ReadLockLedger()
	defer m.ReadUnlockLedger()

	return m.ReadOutputByOutputIDWithoutLocking(outputID)
}

// code guards.
var _ kvStorable = &Output{}
