package mempooltests

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance    mempool.MemPool[vote.MockedRank]
	ConflictDAG conflictdag.ConflictDAG[iotago.TransactionID, mempool.StateID, vote.MockedRank]

	referencesByAlias        map[string]iotago.Input
	stateIDByAlias           map[string]mempool.StateID
	signedTransactionByAlias map[string]mempool.SignedTransaction
	transactionByAlias       map[string]mempool.Transaction
	blockIDsByAlias          map[string]iotago.BlockID

	ledgerState *ledgertests.MockStateResolver
	workers     *workerpool.Group

	test  *testing.T
	mutex syncutils.RWMutex
}

func NewTestFramework(test *testing.T, instance mempool.MemPool[vote.MockedRank], conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, mempool.StateID, vote.MockedRank], ledgerState *ledgertests.MockStateResolver, workers *workerpool.Group) *TestFramework {
	t := &TestFramework{
		Instance:                 instance,
		ConflictDAG:              conflictDAG,
		referencesByAlias:        make(map[string]iotago.Input),
		stateIDByAlias:           make(map[string]mempool.StateID),
		signedTransactionByAlias: make(map[string]mempool.SignedTransaction),
		transactionByAlias:       make(map[string]mempool.Transaction),
		blockIDsByAlias:          make(map[string]iotago.BlockID),

		ledgerState: ledgerState,
		workers:     workers,
		test:        test,
	}

	t.setupHookedEvents()

	return t
}
func (t *TestFramework) CreateSignedTransaction(transactionAlias string, referencedContextStates, referencedStates []string, outputCount uint16, invalid ...bool) {
	t.CreateTransaction(transactionAlias, referencedContextStates, referencedStates, outputCount, invalid...)
	t.SignedTransactionFromTransaction(transactionAlias+"-signed", transactionAlias)
}
func (t *TestFramework) SignedTransactionFromTransaction(signedTransactionAlias string, transactionAlias string) {
	transaction, exists := t.transactionByAlias[transactionAlias]
	require.True(t.test, exists, "transaction with alias %s does not exist", transactionAlias)

	// create transaction
	signedTransaction := NewSignedTransaction(transaction)

	t.signedTransactionByAlias[signedTransactionAlias] = signedTransaction

	// register the transaction ID alias
	signedTransactionID, signedTransactionIDErr := signedTransaction.ID()
	require.NoError(t.test, signedTransactionIDErr, "failed to retrieve signed transaction ID of signed transaction with alias '%s'", signedTransactionAlias)
	signedTransactionID.RegisterAlias(signedTransactionAlias)
}

func (t *TestFramework) CreateTransaction(alias string, referencedContextStates, referencedStates []string, outputCount uint16, invalid ...bool) {
	// create transaction
	transaction := NewTransaction(outputCount, lo.Map(referencedStates, t.stateReference), lo.Map(referencedContextStates, t.stateContextReference)...)
	transaction.invalidTransaction = len(invalid) > 0 && invalid[0]

	t.transactionByAlias[alias] = transaction

	// register the transaction ID alias
	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to retrieve transaction ID of transaction with alias '%s'", alias)
	transactionID.RegisterAlias(alias)

	// register the aliases for the generated output IDs
	for i := uint16(0); i < transaction.outputCount; i++ {
		t.referencesByAlias[alias+":"+strconv.Itoa(int(i))] = &iotago.UTXOInput{
			TransactionID:          transactionID,
			TransactionOutputIndex: i,
		}

		t.stateIDByAlias[alias+":"+strconv.Itoa(int(i))] = t.referencesByAlias[alias+":"+strconv.Itoa(int(i))].StateID()
	}
}

func (t *TestFramework) MarkAttachmentIncluded(alias string) bool {
	return t.Instance.MarkAttachmentIncluded(t.BlockID(alias))
}

func (t *TestFramework) BlockID(alias string) iotago.BlockID {
	blockID, exists := t.blockIDsByAlias[alias]
	require.True(t.test, exists, "block ID with alias '%s' does not exist", alias)

	return blockID
}

func (t *TestFramework) AttachTransactions(transactionAlias ...string) error {
	for _, alias := range transactionAlias {
		if err := t.AttachTransaction(alias, alias, alias, 1); err != nil {
			return err
		}
	}

	return nil
}

func (t *TestFramework) AttachTransaction(signedTransactionAlias, transactionAlias, blockAlias string, slot iotago.SlotIndex) error {
	signedTransaction, signedTransactionExists := t.signedTransactionByAlias[signedTransactionAlias]
	require.True(t.test, signedTransactionExists, "signedTransaction with alias '%s' does not exist", signedTransactionAlias)

	transaction, transactionExists := t.transactionByAlias[transactionAlias]
	require.True(t.test, transactionExists, "transaction with alias '%s' does not exist", transactionAlias)

	t.blockIDsByAlias[blockAlias] = iotago.SlotIdentifierRepresentingData(slot, []byte(blockAlias))
	t.blockIDsByAlias[blockAlias].RegisterAlias(blockAlias)

	if _, err := t.Instance.AttachSignedTransaction(signedTransaction, transaction, t.blockIDsByAlias[blockAlias]); err != nil {
		return err
	}

	return nil
}

func (t *TestFramework) CommitSlot(slot iotago.SlotIndex) {
	stateDiff := t.Instance.StateDiff(slot)

	stateDiff.CreatedStates().ForEach(func(_ mempool.StateID, state mempool.StateMetadata) bool {
		t.ledgerState.AddOutputState(state.State())

		return true
	})

	stateDiff.DestroyedStates().ForEach(func(stateID mempool.StateID, metadata mempool.StateMetadata) bool {
		t.ledgerState.DestroyOutputState(stateID)

		return true
	})

	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, transaction mempool.TransactionMetadata) bool {
		transaction.Commit()

		return true
	})
}

func (t *TestFramework) TransactionMetadata(alias string) (mempool.TransactionMetadata, bool) {
	return t.Instance.TransactionMetadata(t.TransactionID(alias))
}

func (t *TestFramework) TransactionMetadataByAttachment(alias string) (mempool.TransactionMetadata, bool) {
	return t.Instance.TransactionMetadataByAttachment(t.BlockID(alias))
}

func (t *TestFramework) OutputStateMetadata(alias string) (mempool.StateMetadata, error) {
	return t.Instance.StateMetadata(t.stateReference(alias))
}

func (t *TestFramework) StateID(alias string) mempool.StateID {
	if alias == "genesis" {
		return (&iotago.UTXOInput{}).StateID()
	}

	stateID, exists := t.stateIDByAlias[alias]
	require.True(t.test, exists, "StateID with alias '%s' does not exist", alias)

	return stateID
}

func (t *TestFramework) SignedTransactionID(alias string) iotago.SignedTransactionID {
	signedTransaction, signedTransactionExists := t.signedTransactionByAlias[alias]
	require.True(t.test, signedTransactionExists, "transaction with alias '%s' does not exist", alias)

	signedTransactionID, signedTransactionIDErr := signedTransaction.ID()
	require.NoError(t.test, signedTransactionIDErr, "failed to retrieve signed transaction ID of signed transaction with alias '%s'", alias)

	return signedTransactionID
}

func (t *TestFramework) TransactionID(alias string) iotago.TransactionID {
	transaction, transactionExists := t.transactionByAlias[alias]
	require.True(t.test, transactionExists, "transaction with alias '%s' does not exist", alias)

	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to retrieve transaction ID of transaction with alias '%s'", alias)

	return transactionID
}

func (t *TestFramework) RequireBooked(transactionAliases ...string) {
	t.waitBooked(transactionAliases...)

	t.requireMarkedBooked(transactionAliases...)
}

func (t *TestFramework) RequireAccepted(transactionAliases map[string]bool) {
	for transactionAlias, accepted := range transactionAliases {
		transactionMetadata, transactionMetadataExists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))

		require.True(t.test, transactionMetadataExists, "transaction %s should exist", transactionAlias)
		require.Equal(t.test, accepted, transactionMetadata.IsAccepted(), "transaction %s was incorrectly accepted", transactionAlias)
	}
}

func (t *TestFramework) RequireInvalid(transactionAliases ...string) {
	t.waitInvalid(transactionAliases...)

	for _, transactionAlias := range transactionAliases {
		transactionMetadata, transactionMetadataExists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))

		require.True(t.test, transactionMetadataExists, "transaction %s should exist", transactionAlias)
		require.True(t.test, transactionMetadata.IsInvalid(), "transaction %s was incorrectly accepted", transactionAlias)
	}
}

func (t *TestFramework) RequireTransactionsEvicted(transactionAliases map[string]bool) {
	for transactionAlias, deleted := range transactionAliases {
		_, exists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))
		require.Equal(t.test, deleted, !exists, "transaction %s has incorrect eviction state", transactionAlias)
	}
}

func (t *TestFramework) RequireConflictIDs(conflictMapping map[string][]string) {
	for transactionAlias, conflictAliases := range conflictMapping {
		transactionMetadata, exists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))
		require.True(t.test, exists, "transaction %s does not exist", transactionAlias)

		conflictIDs := transactionMetadata.ConflictIDs()
		require.Equal(t.test, len(conflictAliases), conflictIDs.Size(), "%s has wrong number of ConflictIDs", transactionAlias)

		for _, conflictAlias := range conflictAliases {
			require.True(t.test, conflictIDs.Has(t.TransactionID(conflictAlias)), "transaction %s should have conflict %s, instead had %s", transactionAlias, conflictAlias, conflictIDs)
		}
	}
}

func (t *TestFramework) RequireAttachmentsEvicted(attachmentAliases map[string]bool) {
	for attachmentAlias, deleted := range attachmentAliases {
		_, exists := t.Instance.TransactionMetadataByAttachment(t.BlockID(attachmentAlias))
		require.Equal(t.test, deleted, !exists, "attachment %s has incorrect eviction state", attachmentAlias)
	}
}

func (t *TestFramework) setupHookedEvents() {
	t.Instance.OnTransactionAttached(func(metadata mempool.TransactionMetadata) {
		if debug.GetEnabled() {
			t.test.Logf("[TRIGGERED] mempool.TransactionAttached with '%s'", metadata.ID())
		}

		metadata.OnSolid(func() {
			if debug.GetEnabled() {
				t.test.Logf("[TRIGGERED] mempool.Events.TransactionSolid with '%s'", metadata.ID())
			}

			require.True(t.test, metadata.IsSolid(), "transaction is not marked as solid")
		})

		metadata.OnExecuted(func() {
			if debug.GetEnabled() {
				t.test.Logf("[TRIGGERED] mempool.Events.TransactionExecuted with '%s'", metadata.ID())
			}

			require.True(t.test, metadata.IsExecuted(), "transaction is not marked as executed")
		})

		metadata.OnBooked(func() {
			if debug.GetEnabled() {
				t.test.Logf("[TRIGGERED] mempool.Events.TransactionBooked with '%s'", metadata.ID())
			}

			require.True(t.test, metadata.IsBooked(), "transaction is not marked as booked")
		})

		metadata.OnAccepted(func() {
			if debug.GetEnabled() {
				t.test.Logf("[TRIGGERED] mempool.Events.TransactionAccepted with '%s'", metadata.ID())
			}

			require.True(t.test, metadata.IsAccepted(), "transaction is not marked as accepted")
		})

		metadata.OnPending(func() {
			if debug.GetEnabled() {
				t.test.Logf("[TRIGGERED] mempool.Events.TransactionPending with '%s'", metadata.ID())
			}

			require.True(t.test, metadata.IsPending(), "transaction is not marked as pending")
		})

		metadata.OnPending(func() {
			//	if debug.GetEnabled() {
			//		t.test.Logf("[TRIGGERED] mempool.Events.TransactionAccepted with '%s'", metadata.ID())
			//	}
			//
			//	require.False(t.test, metadata.IsAccepted(), "transaction is not marked as pending")
			//
			//	t.markTransactionAcceptedTriggered(metadata.ID(), true)
		})
	})
}

func (t *TestFramework) stateContextReference(alias string) iotago.Input {
	return &iotago.CommitmentInput{CommitmentID: iotago.SlotIdentifierRepresentingData(0, []byte(alias))}
}

func (t *TestFramework) stateReference(alias string) iotago.Input {
	if alias == "genesis" {
		return &iotago.UTXOInput{}
	}

	reference, exists := t.referencesByAlias[alias]
	require.True(t.test, exists, "reference with alias '%s' does not exist", alias)

	return reference
}

func (t *TestFramework) waitBooked(transactionAliases ...string) {
	var allBooked sync.WaitGroup

	allBooked.Add(len(transactionAliases))
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, exists := t.TransactionMetadata(transactionAlias)
		require.True(t.test, exists, "transaction '%s' does not exist", transactionAlias)

		transactionMetadata.OnBooked(allBooked.Done)
	}

	allBooked.Wait()
}

func (t *TestFramework) waitInvalid(transactionAliases ...string) {
	var allInvalid sync.WaitGroup

	allInvalid.Add(len(transactionAliases))
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, exists := t.TransactionMetadata(transactionAlias)
		require.True(t.test, exists, "transaction '%s' does not exist", transactionAlias)

		transactionMetadata.OnInvalid(func(_ error) {
			allInvalid.Done()
		})
	}

	allInvalid.Wait()
}

func (t *TestFramework) requireMarkedBooked(transactionAliases ...string) {
	for _, transactionAlias := range transactionAliases {
		transactionMetadata, transactionMetadataExists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))

		require.True(t.test, transactionMetadataExists, "transaction %s should exist", transactionAlias)
		require.True(t.test, transactionMetadata.IsBooked(), "transaction %s was not booked", transactionAlias)
	}
}

func (t *TestFramework) AssertStateDiff(slot iotago.SlotIndex, spentOutputAliases, createdOutputAliases, transactionAliases []string) {
	stateDiff := t.Instance.StateDiff(slot)

	require.Equal(t.test, len(spentOutputAliases), stateDiff.DestroyedStates().Size())
	require.Equal(t.test, len(createdOutputAliases), stateDiff.CreatedStates().Size())
	require.Equal(t.test, len(transactionAliases), stateDiff.ExecutedTransactions().Size())
	require.Equal(t.test, len(transactionAliases), stateDiff.Mutations().Size())

	for _, transactionAlias := range transactionAliases {
		require.True(t.test, stateDiff.ExecutedTransactions().Has(t.TransactionID(transactionAlias)), "transaction %s was not executed", transactionAlias)
		require.True(t.test, lo.PanicOnErr(stateDiff.Mutations().Has(t.TransactionID(transactionAlias))), "transaction %s was not mutated", transactionAlias)
	}

	for _, createdOutputAlias := range createdOutputAliases {
		require.Truef(t.test, stateDiff.CreatedStates().Has(t.StateID(createdOutputAlias)), "state %s was not created", createdOutputAlias)
	}

	for _, spentOutputAlias := range spentOutputAliases {
		require.Truef(t.test, stateDiff.DestroyedStates().Has(t.StateID(spentOutputAlias)), "state %s was not destroyed", spentOutputAlias)
	}
}

func (t *TestFramework) WaitChildren() {
	t.workers.WaitChildren()
}

func (t *TestFramework) Cleanup() {
	t.workers.WaitChildren()
	t.ledgerState.Cleanup()

	iotago.UnregisterIdentifierAliases()

	t.referencesByAlias = make(map[string]iotago.Input)
	t.stateIDByAlias = make(map[string]mempool.StateID)
	t.transactionByAlias = make(map[string]mempool.Transaction)
	t.signedTransactionByAlias = make(map[string]mempool.SignedTransaction)
	t.blockIDsByAlias = make(map[string]iotago.BlockID)
}
