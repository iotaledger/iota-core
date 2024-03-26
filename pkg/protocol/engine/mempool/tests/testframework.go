package mempooltests

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/debug"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	ledgertests "github.com/iotaledger/iota-core/pkg/protocol/engine/ledger/tests"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TestFramework struct {
	Instance mempool.MemPool[vote.MockedRank]
	SpendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, vote.MockedRank]

	referencesByAlias        map[string]mempool.StateReference
	stateIDByAlias           map[string]mempool.StateID
	signedTransactionByAlias map[string]mempool.SignedTransaction
	transactionByAlias       map[string]mempool.Transaction
	blockIDsByAlias          map[string]iotago.BlockID

	ledgerState *ledgertests.MockStateResolver

	test *testing.T
}

func NewTestFramework(t *testing.T, instance mempool.MemPool[vote.MockedRank], spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, vote.MockedRank], ledgerState *ledgertests.MockStateResolver) *TestFramework {
	t.Helper()

	tf := &TestFramework{
		Instance:                 instance,
		SpendDAG:                 spendDAG,
		referencesByAlias:        make(map[string]mempool.StateReference),
		stateIDByAlias:           make(map[string]mempool.StateID),
		signedTransactionByAlias: make(map[string]mempool.SignedTransaction),
		transactionByAlias:       make(map[string]mempool.Transaction),
		blockIDsByAlias:          make(map[string]iotago.BlockID),

		ledgerState: ledgerState,
		test:        t,
	}

	tf.setupHookedEvents()

	return tf
}

func (t *TestFramework) InjectState(alias string, state mempool.State) {
	t.referencesByAlias[alias] = NewStateReference(state.StateID(), state.Type())

	t.ledgerState.AddOutputState(state)
}

func (t *TestFramework) CreateSignedTransaction(transactionAlias string, referencedStates []string, outputCount uint16, invalid ...bool) {
	t.CreateTransaction(transactionAlias, referencedStates, outputCount, invalid...)
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

func (t *TestFramework) CreateTransaction(alias string, referencedStates []string, outputCount uint16, invalid ...bool) {
	// create transaction
	transaction := NewTransaction(outputCount, lo.Map(referencedStates, t.stateReference)...)
	transaction.invalidTransaction = len(invalid) > 0 && invalid[0]

	t.transactionByAlias[alias] = transaction

	// register the transaction ID alias
	transactionID, transactionIDErr := transaction.ID()
	require.NoError(t.test, transactionIDErr, "failed to retrieve transaction ID of transaction with alias '%s'", alias)
	transactionID.RegisterAlias(alias)

	// register the aliases for the generated output IDs
	for i := range transaction.outputCount {
		t.referencesByAlias[alias+":"+strconv.Itoa(int(i))] = mempool.UTXOInputStateRefFromInput(&iotago.UTXOInput{
			TransactionID:          transactionID,
			TransactionOutputIndex: i,
		})

		t.stateIDByAlias[alias+":"+strconv.Itoa(int(i))] = t.referencesByAlias[alias+":"+strconv.Itoa(int(i))].ReferencedStateID()
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

	t.blockIDsByAlias[blockAlias] = iotago.BlockIDRepresentingData(slot, []byte(blockAlias))
	t.blockIDsByAlias[blockAlias].RegisterAlias(blockAlias)

	if _, err := t.Instance.AttachSignedTransaction(signedTransaction, transaction, t.blockIDsByAlias[blockAlias]); err != nil {
		return err
	}

	return nil
}

func (t *TestFramework) CommitSlot(slot iotago.SlotIndex) {
	stateDiff, err := t.Instance.CommitStateDiff(slot)
	if err != nil {
		panic(err)
	}

	stateDiff.CreatedStates().ForEach(func(_ mempool.StateID, state mempool.StateMetadata) bool {
		t.ledgerState.AddOutputState(state.State())

		return true
	})

	stateDiff.DestroyedStates().ForEach(func(stateID mempool.StateID, _ mempool.StateMetadata) bool {
		t.ledgerState.DestroyOutputState(stateID)

		return true
	})

	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, transaction mempool.TransactionMetadata) bool {
		transaction.Commit()

		return true
	})

	t.Instance.Evict(slot)
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
		return iotago.IdentifierFromData(lo.PanicOnErr((&iotago.UTXOInput{}).OutputID().Bytes()))
	}

	stateID, exists := t.stateIDByAlias[alias]
	require.True(t.test, exists, "ReferencedStateID with alias '%s' does not exist", alias)

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

func (t *TestFramework) RequireSpenderIDs(spendMapping map[string][]string) {
	for transactionAlias, spendAliases := range spendMapping {
		transactionMetadata, exists := t.Instance.TransactionMetadata(t.TransactionID(transactionAlias))
		require.True(t.test, exists, "transaction %s does not exist", transactionAlias)

		spenderIDs := transactionMetadata.SpenderIDs()
		require.Equal(t.test, len(spendAliases), spenderIDs.Size(), "%s has wrong number of SpenderIDs", transactionAlias)

		for _, spendAlias := range spendAliases {
			require.True(t.test, spenderIDs.Has(t.TransactionID(spendAlias)), "transaction %s should have spender %s, instead had %s", transactionAlias, spendAlias, spenderIDs)
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
	})
}

func (t *TestFramework) stateReference(alias string) mempool.StateReference {
	if alias == "genesis" {
		return mempool.UTXOInputStateRefFromInput(&iotago.UTXOInput{})
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
	stateDiff, err := t.Instance.CommitStateDiff(slot)
	require.NoError(t.test, err)

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

func (t *TestFramework) Cleanup() {
	t.ledgerState.Cleanup()

	iotago.UnregisterIdentifierAliases()

	t.referencesByAlias = make(map[string]mempool.StateReference)
	t.stateIDByAlias = make(map[string]mempool.StateID)
	t.transactionByAlias = make(map[string]mempool.Transaction)
	t.signedTransactionByAlias = make(map[string]mempool.SignedTransaction)
	t.blockIDsByAlias = make(map[string]iotago.BlockID)
}

type genericReference struct {
	referencedStateID iotago.Identifier
	stateType         mempool.StateType
}

func NewStateReference(referencedStateID iotago.Identifier, stateType mempool.StateType) mempool.StateReference {
	return &genericReference{
		referencedStateID: referencedStateID,
		stateType:         stateType,
	}
}

func (g *genericReference) ReferencedStateID() iotago.Identifier {
	return g.referencedStateID
}

func (g *genericReference) Type() mempool.StateType {
	return g.stateType
}
