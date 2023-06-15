package utxoledger

import (
	"bytes"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/serializer/v2/marshalutil"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SpentConsumer is a function that consumes a spent output.
// Returning false from this function indicates to abort the iteration.
type SpentConsumer func(spent *Spent) bool

// LexicalOrderedSpents are spents ordered in lexical order by their outputID.
type LexicalOrderedSpents []*Spent

func (l LexicalOrderedSpents) Len() int {
	return len(l)
}

func (l LexicalOrderedSpents) Less(i, j int) bool {
	return bytes.Compare(l[i].outputID[:], l[j].outputID[:]) < 0
}

func (l LexicalOrderedSpents) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

// Spent are already spent TXOs (transaction outputs).
type Spent struct {
	outputID iotago.OutputID
	// the ID of the transaction that spent the output
	transactionIDSpent iotago.TransactionID
	// the index of the slot that spent the output
	slotIndexSpent iotago.SlotIndex

	output *Output
}

func (s *Spent) Output() *Output {
	return s.output
}

func (s *Spent) OutputID() iotago.OutputID {
	return s.outputID
}

func (s *Spent) MapKey() string {
	return string(s.outputID[:])
}

func (s *Spent) BlockID() iotago.BlockID {
	return s.output.BlockID()
}

func (s *Spent) OutputType() iotago.OutputType {
	return s.output.OutputType()
}

func (s *Spent) Deposit() uint64 {
	return s.output.Deposit()
}

// TransactionIDSpent returns the ID of the transaction that spent the output.
func (s *Spent) TransactionIDSpent() iotago.TransactionID {
	return s.transactionIDSpent
}

// SlotIndexSpent returns the index of the slot that spent the output.
func (s *Spent) SlotIndexSpent() iotago.SlotIndex {
	return s.slotIndexSpent
}

type Spents []*Spent

func NewSpent(output *Output, transactionIDSpent iotago.TransactionID, slotIndexSpent iotago.SlotIndex) *Spent {
	return &Spent{
		outputID:           output.outputID,
		output:             output,
		transactionIDSpent: transactionIDSpent,
		slotIndexSpent:     slotIndexSpent,
	}
}

func spentStorageKeyForOutputID(outputID iotago.OutputID) []byte {
	ms := marshalutil.New(35)
	ms.WriteByte(StoreKeyPrefixOutputSpent) // 1 byte
	ms.WriteBytes(outputID[:])              // 34 bytes

	return ms.Bytes()
}

func (s *Spent) KVStorableKey() (key []byte) {
	return spentStorageKeyForOutputID(s.outputID)
}

func (s *Spent) KVStorableValue() (value []byte) {
	ms := marshalutil.New(48)
	ms.WriteBytes(s.transactionIDSpent[:])  // 32 bytes
	ms.WriteBytes(s.slotIndexSpent.Bytes()) // 8 bytes

	return ms.Bytes()
}

func (s *Spent) kvStorableLoad(_ *Manager, key []byte, value []byte) error {
	// Parse key
	keyUtil := marshalutil.New(key)

	// Read prefix output
	_, err := keyUtil.ReadByte()
	if err != nil {
		return err
	}

	// Read OutputID
	if s.outputID, err = ParseOutputID(keyUtil); err != nil {
		return err
	}

	// Parse value
	valueUtil := marshalutil.New(value)

	// Read transaction ID
	if s.transactionIDSpent, err = parseTransactionID(valueUtil); err != nil {
		return err
	}

	// Read milestone index
	s.slotIndexSpent, err = parseSlotIndex(valueUtil)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) loadOutputOfSpent(s *Spent) error {
	output, err := m.ReadOutputByOutputIDWithoutLocking(s.outputID)
	if err != nil {
		return err
	}
	s.output = output

	return nil
}

func (m *Manager) ReadSpentForOutputIDWithoutLocking(outputID iotago.OutputID) (*Spent, error) {
	output, err := m.ReadOutputByOutputIDWithoutLocking(outputID)
	if err != nil {
		return nil, err
	}

	key := spentStorageKeyForOutputID(outputID)
	value, err := m.store.Get(key)
	if err != nil {
		return nil, err
	}

	spent := &Spent{}
	if err := spent.kvStorableLoad(m, key, value); err != nil {
		return nil, err
	}

	spent.output = output

	return spent, nil
}

func storeSpent(spent *Spent, mutations kvstore.BatchedMutations) error {
	return mutations.Set(spent.KVStorableKey(), spent.KVStorableValue())
}

func deleteSpent(spent *Spent, mutations kvstore.BatchedMutations) error {
	return mutations.Delete(spent.KVStorableKey())
}

// code guards.
var _ kvStorable = &Spent{}
