package utxoledger

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ErrOutputsSumNotEqualTotalSupply is returned if the sum of the output deposits is not equal the total supply of tokens.
var ErrOutputsSumNotEqualTotalSupply = errors.New("accumulated output balance is not equal to total supply")

type Manager struct {
	store     kvstore.KVStore
	storeLock sync.RWMutex

	stateTree *ads.Map[iotago.OutputID, *stateTreeMetadata]

	apiProvider permanent.APIBySlotIndexProviderFunc
}

func New(store kvstore.KVStore, apiProvider permanent.APIBySlotIndexProviderFunc) *Manager {
	return &Manager{
		store: store,
		stateTree: ads.NewMap(lo.PanicOnErr(store.WithExtendedRealm(kvstore.Realm{StoreKeyPrefixStateTree})),
			iotago.OutputID.Bytes,
			iotago.OutputIDFromBytes,
			(*stateTreeMetadata).Bytes,
			stateMetadataFromBytes,
		),
		apiProvider: apiProvider,
	}
}

func (m *Manager) API(slot iotago.SlotIndex) iotago.API {
	return m.apiProvider(slot)
}

// KVStore returns the underlying KVStore.
func (m *Manager) KVStore() kvstore.KVStore {
	return m.store
}

// ClearLedgerState removes all entries from the ledger (spent, unspent, diff).
func (m *Manager) ClearLedgerState() (err error) {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	defer func() {
		if errFlush := m.store.Flush(); err == nil && errFlush != nil {
			err = errFlush
		}
	}()

	return m.store.Clear()
}

func (m *Manager) ReadLockLedger() {
	m.storeLock.RLock()
}

func (m *Manager) ReadUnlockLedger() {
	m.storeLock.RUnlock()
}

func (m *Manager) WriteLockLedger() {
	m.storeLock.Lock()
}

func (m *Manager) WriteUnlockLedger() {
	m.storeLock.Unlock()
}

func (m *Manager) PruneSlotIndexWithoutLocking(index iotago.SlotIndex) error {
	diff, err := m.SlotDiffWithoutLocking(index)
	if err != nil {
		return err
	}

	mutations, err := m.store.Batched()
	if err != nil {
		return err
	}

	for _, spent := range diff.Spents {
		if err := deleteOutput(spent.output, mutations); err != nil {
			mutations.Cancel()

			return err
		}

		if err := deleteSpent(spent, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	if err := deleteDiff(index, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	return mutations.Commit()
}

func storeLedgerIndex(index iotago.SlotIndex, mutations kvstore.BatchedMutations) error {
	return mutations.Set([]byte{StoreKeyPrefixLedgerSlotIndex}, index.MustBytes())
}

func (m *Manager) StoreLedgerIndexWithoutLocking(index iotago.SlotIndex) error {
	return m.store.Set([]byte{StoreKeyPrefixLedgerSlotIndex}, index.MustBytes())
}

func (m *Manager) StoreLedgerIndex(index iotago.SlotIndex) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	return m.StoreLedgerIndexWithoutLocking(index)
}

func (m *Manager) ReadLedgerIndexWithoutLocking() (iotago.SlotIndex, error) {
	value, err := m.store.Get([]byte{StoreKeyPrefixLedgerSlotIndex})
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			// there is no ledger milestone yet => return 0
			return 0, nil
		}

		return 0, fmt.Errorf("failed to load ledger milestone index: %w", err)
	}

	return lo.DropCount(iotago.SlotIndexFromBytes(value))
}

func (m *Manager) ReadLedgerIndex() (iotago.SlotIndex, error) {
	m.ReadLockLedger()
	defer m.ReadUnlockLedger()

	return m.ReadLedgerIndexWithoutLocking()
}

func (m *Manager) ApplyDiffWithoutLocking(index iotago.SlotIndex, newOutputs Outputs, newSpents Spents) error {
	mutations, err := m.store.Batched()
	if err != nil {
		return err
	}

	for _, output := range newOutputs {
		if err = storeOutput(output, mutations); err != nil {
			mutations.Cancel()

			return err
		}
		if err = markAsUnspent(output, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	for _, spent := range newSpents {
		if err = storeSpentAndMarkOutputAsSpent(spent, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	msDiff := &SlotDiff{
		Index:   index,
		Outputs: newOutputs,
		Spents:  newSpents,
	}

	if err = storeDiff(msDiff, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err = storeLedgerIndex(index, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err = mutations.Commit(); err != nil {
		return err
	}

	for _, output := range newOutputs {
		m.stateTree.Set(output.OutputID(), newStateMetadata(output))
	}
	for _, spent := range newSpents {
		m.stateTree.Delete(spent.OutputID())
	}

	return nil
}

func (m *Manager) ApplyDiff(index iotago.SlotIndex, newOutputs Outputs, newSpents Spents) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	return m.ApplyDiffWithoutLocking(index, newOutputs, newSpents)
}

func (m *Manager) RollbackDiffWithoutLocking(index iotago.SlotIndex, newOutputs Outputs, newSpents Spents) error {
	mutations, err := m.store.Batched()
	if err != nil {
		return err
	}

	// we have to store the spents as output and mark them as unspent
	for _, spent := range newSpents {
		if err := storeOutput(spent.output, mutations); err != nil {
			mutations.Cancel()

			return err
		}

		if err := deleteSpentAndMarkOutputAsUnspent(spent, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	// we have to delete the newOutputs of this milestone
	for _, output := range newOutputs {
		if err := deleteOutput(output, mutations); err != nil {
			mutations.Cancel()

			return err
		}
		if err := deleteOutputLookups(output, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	if err := deleteDiff(index, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := storeLedgerIndex(index-1, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := mutations.Commit(); err != nil {
		return err
	}

	for _, spent := range newSpents {
		m.stateTree.Set(spent.OutputID(), newStateMetadata(spent.Output()))
	}
	for _, output := range newOutputs {
		m.stateTree.Delete(output.OutputID())
	}

	return nil
}

func (m *Manager) RollbackDiff(index iotago.SlotIndex, newOutputs Outputs, newSpents Spents) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	return m.RollbackDiffWithoutLocking(index, newOutputs, newSpents)
}

func (m *Manager) CheckLedgerState(tokenSupply iotago.BaseToken) error {
	total, _, err := m.ComputeLedgerBalance()
	if err != nil {
		return err
	}

	if total != tokenSupply {
		return ErrOutputsSumNotEqualTotalSupply
	}

	return nil
}

func (m *Manager) AddUnspentOutputWithoutLocking(unspentOutput *Output) error {
	mutations, err := m.store.Batched()
	if err != nil {
		return err
	}

	if err := storeOutput(unspentOutput, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := markAsUnspent(unspentOutput, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := mutations.Commit(); err != nil {
		return err
	}

	m.stateTree.Set(unspentOutput.OutputID(), newStateMetadata(unspentOutput))

	return nil
}

func (m *Manager) AddUnspentOutput(unspentOutput *Output) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	return m.AddUnspentOutputWithoutLocking(unspentOutput)
}

func (m *Manager) LedgerStateSHA256Sum() ([]byte, error) {
	m.ReadLockLedger()
	defer m.ReadUnlockLedger()

	ledgerStateHash := sha256.New()

	ledgerIndex, err := m.ReadLedgerIndexWithoutLocking()
	if err != nil {
		return nil, err
	}
	if err := binary.Write(ledgerStateHash, binary.LittleEndian, ledgerIndex); err != nil {
		return nil, err
	}

	// get all UTXOs and sort them by outputID
	outputIDs, err := m.UnspentOutputsIDs(ReadLockLedger(false))
	if err != nil {
		return nil, err
	}

	for _, outputID := range outputIDs.RemoveDupsAndSort() {
		output, err := m.ReadOutputByOutputID(outputID)
		if err != nil {
			return nil, err
		}

		if _, err := ledgerStateHash.Write(output.outputID[:]); err != nil {
			return nil, err
		}

		if _, err := ledgerStateHash.Write(output.KVStorableValue()); err != nil {
			return nil, err
		}
	}

	// Add root of the state tree
	stateTreeBytes, err := m.StateTreeRoot().Bytes()
	if err != nil {
		return nil, err
	}

	if _, err := ledgerStateHash.Write(stateTreeBytes); err != nil {
		return nil, err
	}

	// calculate sha256 hash
	return ledgerStateHash.Sum(nil), nil
}
