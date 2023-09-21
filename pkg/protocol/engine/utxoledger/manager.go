package utxoledger

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ErrOutputsSumNotEqualTotalSupply is returned if the sum of the output base token amounts is not equal the total supply of tokens.
var ErrOutputsSumNotEqualTotalSupply = ierrors.New("accumulated output balance is not equal to total supply")

type Manager struct {
	store     kvstore.KVStore
	storeLock syncutils.RWMutex

	stateTree ads.Map[iotago.OutputID, *stateTreeMetadata]

	apiProvider iotago.APIProvider
}

func New(store kvstore.KVStore, apiProvider iotago.APIProvider) *Manager {
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
		// There's no need to prune this slot.
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil
		}

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
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			// there is no ledger milestone yet => return 0
			return 0, nil
		}

		return 0, ierrors.Errorf("failed to load ledger milestone index: %w", err)
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
		if err := markAsUnspent(output, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	for _, spent := range newSpents {
		if err := storeSpentAndMarkOutputAsSpent(spent, mutations); err != nil {
			mutations.Cancel()

			return err
		}
	}

	slotDiff := &SlotDiff{
		Index:   index,
		Outputs: newOutputs,
		Spents:  newSpents,
	}

	if err := storeDiff(slotDiff, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := storeLedgerIndex(index, mutations); err != nil {
		mutations.Cancel()

		return err
	}

	if err := mutations.Commit(); err != nil {
		return err
	}

	for _, output := range newOutputs {
		if err := m.stateTree.Set(output.OutputID(), newStateMetadata(output)); err != nil {
			return ierrors.Wrapf(err, "failed to set new oputput in state tree, outputID: %s", output.OutputID())
		}
	}
	for _, spent := range newSpents {
		if _, err := m.stateTree.Delete(spent.OutputID()); err != nil {
			return ierrors.Wrapf(err, "failed to delete spent output from state tree, outputID: %s", spent.OutputID())
		}
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "failed to commit state tree")
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
		if err := m.stateTree.Set(spent.OutputID(), newStateMetadata(spent.Output())); err != nil {
			return ierrors.Wrapf(err, "failed to set new spent output in state tree, outputID: %s", spent.OutputID())
		}
	}
	for _, output := range newOutputs {
		if _, err := m.stateTree.Delete(output.OutputID()); err != nil {
			return ierrors.Wrapf(err, "failed to delete new output from state tree, outputID: %s", output.OutputID())
		}
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "failed to commit state tree")
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

func (m *Manager) AddGenesisUnspentOutputWithoutLocking(unspentOutput *Output) error {
	if err := m.importUnspentOutputWithoutLocking(unspentOutput); err != nil {
		return ierrors.Wrapf(err, "failed to import unspent output, outputID: %s", unspentOutput.OutputID())
	}

	if err := m.stateTree.Commit(); err != nil {
		return ierrors.Wrap(err, "failed to commit state tree")
	}

	return nil
}

func (m *Manager) importUnspentOutputWithoutLocking(unspentOutput *Output) error {
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

	if err := m.stateTree.Set(unspentOutput.OutputID(), newStateMetadata(unspentOutput)); err != nil {
		return ierrors.Wrapf(err, "failed to set state tree entry for output, outputID: %s", unspentOutput.OutputID())
	}

	return nil
}

func (m *Manager) AddGenesisUnspentOutput(unspentOutput *Output) error {
	m.WriteLockLedger()
	defer m.WriteUnlockLedger()

	return m.AddGenesisUnspentOutputWithoutLocking(unspentOutput)
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
		output, err := m.ReadOutputByOutputIDWithoutLocking(outputID)
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
