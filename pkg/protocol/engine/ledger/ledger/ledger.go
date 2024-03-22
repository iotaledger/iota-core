package ledger

import (
	"io"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/mana"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/congestioncontrol/rmc"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag/spenddagv1"
	mempoolv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/slotstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger struct {
	events *ledger.Events

	apiProvider iotago.APIProvider

	utxoLedger       *utxoledger.Manager
	accountsLedger   *accountsledger.Manager
	manaManager      *mana.Manager
	rmcManager       *rmc.Manager
	sybilProtection  sybilprotection.SybilProtection
	commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error)
	memPool          mempool.MemPool[ledger.BlockVoteRank]
	spendDAG         spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]
	errorHandler     func(error)

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, ledger.Ledger] {
	return module.Provide(func(e *engine.Engine) ledger.Ledger {
		l := New(
			e.NewSubModule("Ledger"),
			e.Storage.Ledger(),
			e.Storage.Accounts(),
			e.Storage.Commitments().Load,
			e.BlockCache.Block,
			e.Storage.AccountDiffs,
			e,
			e.SybilProtection,
			e.ErrorHandler("ledger"),
		)

		e.ConstructedEvent().OnTrigger(func() {
			e.Events.Ledger.LinkTo(l.events)
			l.spendDAG = spenddagv1.New[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank](func() int {
				return l.sybilProtection.SeatManager().OnlineCommittee().Size()
			})
			e.Events.SpendDAG.LinkTo(l.spendDAG.Events())

			l.memPool = mempoolv1.New(NewVM(l), l.resolveState, e.Storage.Mutations, e.Workers.CreateGroup("MemPool"), l.spendDAG, l.apiProvider, l.errorHandler)

			l.manaManager = mana.NewManager(l.apiProvider, l.resolveAccountOutput, l.accountsLedger.Account)
			latestCommittedSlot := e.Storage.Settings().LatestCommitment().Slot()
			l.accountsLedger.SetLatestCommittedSlot(latestCommittedSlot)
			l.rmcManager.SetLatestCommittedSlot(latestCommittedSlot)

			e.Events.BlockGadget.BlockPreAccepted.Hook(l.blockPreAccepted)

			// TODO: CHECK IF STILL NECESSARY
			// e.Events.Notarization.SlotCommitted.Hook(func(scd *notarization.SlotCommittedDetails) {
			//	l.memPool.PublishRequestedState(scd.Commitment.Commitment())
			// })

			l.InitializedEvent().Trigger()
		})

		return l
	})
}

func New(
	subModule module.Module,
	utxoLedger *utxoledger.Manager,
	accountsStore kvstore.KVStore,
	commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error),
	blocksFunc func(id iotago.BlockID) (*blocks.Block, bool),
	slotDiffFunc func(iotago.SlotIndex) (*slotstore.AccountDiffs, error),
	apiProvider iotago.APIProvider,
	sybilProtection sybilprotection.SybilProtection,
	errorHandler func(error),
) *Ledger {
	l := &Ledger{
		Module:           subModule,
		events:           ledger.NewEvents(),
		apiProvider:      apiProvider,
		accountsLedger:   accountsledger.New(subModule.NewSubModule("AccountsLedger"), apiProvider, blocksFunc, slotDiffFunc, accountsStore),
		rmcManager:       rmc.NewManager(apiProvider, commitmentLoader),
		utxoLedger:       utxoLedger,
		commitmentLoader: commitmentLoader,
		sybilProtection:  sybilProtection,
		errorHandler:     errorHandler,
		spendDAG: spenddagv1.New[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank](func() int {
			return sybilProtection.SeatManager().OnlineCommittee().Size()
		}),
	}

	l.ShutdownEvent().OnTrigger(l.shutdown)

	l.ConstructedEvent().Trigger()

	return l
}

func (l *Ledger) OnTransactionAttached(handler func(transaction mempool.TransactionMetadata), opts ...event.Option) *event.Hook[func(metadata mempool.TransactionMetadata)] {
	return l.memPool.OnTransactionAttached(handler, opts...)
}

func (l *Ledger) AttachTransaction(block *blocks.Block) (attachedTransaction mempool.SignedTransactionMetadata, containsTransaction bool) {
	if signedTransaction, hasTransaction := block.SignedTransaction(); hasTransaction {
		signedTransactionMetadata, err := l.memPool.AttachSignedTransaction(signedTransaction, signedTransaction.Transaction, block.ID())
		if err != nil {
			l.errorHandler(ierrors.Wrapf(err, "failed to attach signed transaction %s to block %s", signedTransaction.Transaction.MustID(), block.ID()))

			return nil, true
		}

		return signedTransactionMetadata, true
	}

	return nil, false
}

func (l *Ledger) CommitSlot(slot iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, accountRoot iotago.Identifier, created utxoledger.Outputs, consumed utxoledger.Spents, mutations []*iotago.Transaction, err error) {
	ledgerIndex, err := l.utxoLedger.ReadLedgerSlot()
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, err
	}

	if slot != ledgerIndex+1 {
		panic(ierrors.Errorf("there is a gap in the ledgerstate %d vs %d", ledgerIndex+1, slot))
	}

	stateDiff, err := l.memPool.CommitStateDiff(slot)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to retrieve state diff for slot %d", slot)
	}

	// collect outputs and allotments from the "uncompacted" stateDiff
	// outputs need to be processed in the "uncompacted" version of the state diff, as we need to be able to store
	// and retrieve intermediate outputs to show to the user
	spenders, outputs, accountDiffs, err := l.processStateDiffTransactions(stateDiff)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to process state diff transactions in slot %d", slot)
	}

	// Now we process the collected account changes, for that we consume the "compacted" state diff to get the overall
	// account changes at UTXO level without needing to worry about multiple spenders of the same account in the same slot,
	// we only care about the initial account output to be consumed and the final account output to be created.
	// output side
	createdAccounts, consumedAccounts, destroyedAccounts, err := l.processCreatedAndConsumedAccountOutputs(stateDiff, accountDiffs)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to process outputs consumed and created in slot %d", slot)
	}

	l.prepareAccountDiffs(accountDiffs, slot, consumedAccounts, createdAccounts)

	// Commit the changes
	// Update the UTXO ledger
	if err = l.utxoLedger.ApplyDiff(slot, outputs, spenders); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to apply diff to UTXO ledger for slot %d", slot)
	}

	// Update the Accounts ledger
	// first, get the RMC corresponding to this slot
	protocolParams := l.apiProvider.APIForSlot(slot).ProtocolParameters()
	rmcSlot, _ := safemath.SafeSub(slot, protocolParams.MaxCommittableAge()) // We can safely ignore the underflow error and use the default 0 return value
	if rmcSlot < protocolParams.GenesisSlot() {
		rmcSlot = protocolParams.GenesisSlot()
	}
	rmcForSlot, err := l.rmcManager.RMC(rmcSlot)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "ledger failed to get RMC for slot %d", rmcSlot)
	}
	if err = l.accountsLedger.ApplyDiff(slot, rmcForSlot, accountDiffs, destroyedAccounts); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to apply diff to Accounts ledger for slot %d", slot)
	}

	// Update the mana manager's cache
	if err = l.manaManager.ApplyDiff(slot, destroyedAccounts, createdAccounts, accountDiffs); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil, nil, nil, ierrors.Wrapf(err, "failed to apply diff to mana manager for slot %d", slot)
	}

	// Mark each transaction as committed so the mempool can evict it
	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, tx mempool.TransactionMetadata) bool {
		tx.Commit()

		//nolint:forcetypeassert // we only handle a single type of transaction currently
		mutations = append(mutations, tx.Transaction().(*iotago.Transaction))

		return true
	})

	// We evict the mempool before we return, after updating the last committed slot value.
	// We need to evict after updating the last committed slot value because otherwise transactions would be orphaned first
	// (due to removing the attachments) and then committed, which would result in a broken state of the transaction.
	l.memPool.Evict(slot)

	return l.utxoLedger.StateTreeRoot(), stateDiff.Mutations().Root(), l.accountsLedger.AccountsTreeRoot(), outputs, spenders, mutations, nil
}

func (l *Ledger) AddAccount(output *utxoledger.Output, blockIssuanceCredits iotago.BlockIssuanceCredits) error {
	return l.accountsLedger.AddAccount(output, blockIssuanceCredits)
}

func (l *Ledger) AddGenesisUnspentOutput(unspentOutput *utxoledger.Output) error {
	return l.utxoLedger.AddGenesisUnspentOutput(unspentOutput)
}

func (l *Ledger) TrackBlock(block *blocks.Block) {
	l.accountsLedger.TrackBlock(block)

	if _, hasTransaction := block.SignedTransaction(); hasTransaction {
		l.memPool.MarkAttachmentIncluded(block.ID())
	}

	if err := l.rmcManager.BlockAccepted(block); err != nil {
		l.errorHandler(err)
	}
}

func (l *Ledger) Account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error) {
	return l.accountsLedger.Account(accountID, targetIndex)
}

func (l *Ledger) PastAccounts(accountIDs iotago.AccountIDs, targetIndex iotago.SlotIndex) (accountDataMap map[iotago.AccountID]*accounts.AccountData, err error) {
	return l.accountsLedger.PastAccounts(accountIDs, targetIndex)
}

func (l *Ledger) outputFromState(state mempool.State) *utxoledger.Output {
	switch output := state.(type) {
	case *utxoledger.Output:
		// If this output was not booked yet, then it came directly from the mempool, so we need to set the earliest attachment and the booking slot
		if output.SlotBooked() == 0 {
			txWithMetadata, exists := l.memPool.TransactionMetadata(output.OutputID().TransactionID())
			if exists {
				earliestAttachment := txWithMetadata.EarliestIncludedAttachment()

				return output.CopyWithBlockIDAndSlotBooked(earliestAttachment, earliestAttachment.Slot())
			}
		}

		return output
	default:
		panic("unexpected State type")
	}
}

func (l *Ledger) Output(outputID iotago.OutputID) (*utxoledger.Output, error) {
	//nolint:revive //false positive
	if output, spent, err := l.OutputOrSpent(outputID); err != nil {
		return nil, err
	} else if spent != nil {
		return spent.Output(), nil
	} else {
		return output, nil
	}
}

func (l *Ledger) OutputOrSpent(outputID iotago.OutputID) (*utxoledger.Output, *utxoledger.Spent, error) {
	stateRef := mempool.UTXOInputStateRefFromInput(outputID.UTXOInput())
	stateWithMetadata, err := l.memPool.StateMetadata(stateRef)
	if err != nil {
		if ierrors.Is(err, iotago.ErrInputAlreadySpent) {
			l.utxoLedger.ReadLockLedger()
			defer l.utxoLedger.ReadUnlockLedger()

			spent, err := l.utxoLedger.ReadSpentForOutputIDWithoutLocking(outputID)
			if err != nil {
				return nil, nil, err
			}

			return nil, spent, err
		}

		return nil, nil, err
	}

	spender, spent := stateWithMetadata.AcceptedSpender()
	if spent {
		return nil, utxoledger.NewSpent(l.outputFromState(stateWithMetadata.State()), spender.ID(), spender.ID().Slot()), nil
	}

	return l.outputFromState(stateWithMetadata.State()), nil, nil
}

func (l *Ledger) ForEachUnspentOutput(consumer func(output *utxoledger.Output) bool) error {
	return l.utxoLedger.ForEachUnspentOutput(consumer)
}

func (l *Ledger) SlotDiffs(slot iotago.SlotIndex) (*utxoledger.SlotDiff, error) {
	return l.utxoLedger.SlotDiffWithoutLocking(slot)
}

func (l *Ledger) TransactionMetadata(transactionID iotago.TransactionID) (mempool.TransactionMetadata, bool) {
	return l.memPool.TransactionMetadata(transactionID)
}

func (l *Ledger) TransactionMetadataByAttachment(blockID iotago.BlockID) (mempool.TransactionMetadata, bool) {
	return l.memPool.TransactionMetadataByAttachment(blockID)
}

func (l *Ledger) SpendDAG() spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank] {
	return l.spendDAG
}

func (l *Ledger) MemPool() mempool.MemPool[ledger.BlockVoteRank] {
	return l.memPool
}

func (l *Ledger) Import(reader io.ReadSeeker) error {
	if err := l.utxoLedger.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import utxoLedger")
	}

	if err := l.accountsLedger.Import(reader); err != nil {
		return ierrors.Wrap(err, "failed to import accountsLedger")
	}

	return nil
}

func (l *Ledger) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	if err := l.utxoLedger.Export(writer, targetIndex); err != nil {
		return ierrors.Wrap(err, "failed to export utxoLedger")
	}

	if err := l.accountsLedger.Export(writer, targetIndex); err != nil {
		return ierrors.Wrap(err, "failed to export accountsLedger")
	}

	return nil
}

func (l *Ledger) ManaManager() *mana.Manager {
	return l.manaManager
}

func (l *Ledger) RMCManager() *rmc.Manager {
	return l.rmcManager
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (l *Ledger) Reset() {
	// ConflictDAG is evicted automatically when transaction are marked as evicted in the mempool.
	l.memPool.Reset()
	l.accountsLedger.Reset()
	l.rmcManager.Reset()
}

func (l *Ledger) shutdown() {
	l.spendDAG.Shutdown()

	l.StoppedEvent().Trigger()
}

// Process the collected account changes. The consumedAccounts and createdAccounts maps only contain outputs with a
// BIC feature, so allotments made to account without a BIC feature are not tracked here, and they are burned as a result.
// There are 3 possible cases:
// 1. The account was only consumed but not created in this slot, therefore, it is marked as destroyed, and its latest
// state is stored as diff to allow a rollback.
// 2. The account was consumed and created in the same slot, the account was transitioned, and we have to store the
// changes in the diff.
// 3. The account was only created in this slot, in this case we need to track the output's values as the diff.
func (l *Ledger) prepareAccountDiffs(accountDiffs map[iotago.AccountID]*model.AccountDiff, slot iotago.SlotIndex, consumedAccounts map[iotago.AccountID]*utxoledger.Output, createdAccounts map[iotago.AccountID]*utxoledger.Output) {
	for consumedAccountID, consumedOutput := range consumedAccounts {
		// We might have had an allotment on this account, and the diff already exists
		accountDiff := getAccountDiff(accountDiffs, consumedAccountID)

		// Obtain account state at the current latest committed slot, which is slot-1
		accountData, exists, err := l.accountsLedger.Account(consumedAccountID, slot-1)
		if err != nil {
			panic(ierrors.Wrapf(err, "error loading account %s in slot %d", consumedAccountID, slot-1))
		}
		if !exists {
			panic(ierrors.Errorf("could not find destroyed account %s in slot %d", consumedAccountID, slot-1))
		}

		// case 1. the account was destroyed, the diff will be created inside accountLedger on account deletion
		// case 2. the account was transitioned, fill in the diff with the delta information
		// Change and PreviousUpdatedTime are either 0 if we did not have an allotment for this account, or we already
		// have some values from the allotment, so no need to set them explicitly.
		createdOutput, accountTransitioned := createdAccounts[consumedAccountID]
		if !accountTransitioned {
			// case 1.
			continue
		}

		// case 2.
		// created output can never be an implicit account as these can not be transitioned, but consumed output can be.
		switch consumedOutput.Output().Type() {
		case iotago.OutputAccount:
			accountDiff.PreviousExpirySlot = consumedOutput.Output().FeatureSet().BlockIssuer().ExpirySlot
		case iotago.OutputBasic:
			accountDiff.PreviousExpirySlot = iotago.MaxSlotIndex
		}

		accountDiff.PreviousOutputID = consumedOutput.OutputID()
		accountDiff.NewOutputID = createdOutput.OutputID()
		accountDiff.NewExpirySlot = createdOutput.Output().FeatureSet().BlockIssuer().ExpirySlot

		oldPubKeysSet := accountData.BlockIssuerKeys
		newPubKeysSet := iotago.NewBlockIssuerKeys()
		for _, blockIssuerKey := range createdOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys {
			newPubKeysSet.Add(blockIssuerKey)
		}

		// Add public keys that are not in the old set
		accountDiff.BlockIssuerKeysAdded = iotago.NewBlockIssuerKeys()
		for _, newKey := range newPubKeysSet {
			if !oldPubKeysSet.Has(newKey) {
				accountDiff.BlockIssuerKeysAdded.Add(newKey)
			}
		}

		// Remove the keys that are not in the new set
		accountDiff.BlockIssuerKeysRemoved = iotago.NewBlockIssuerKeys()
		for _, oldKey := range oldPubKeysSet {
			if !newPubKeysSet.Has(oldKey) {
				accountDiff.BlockIssuerKeysRemoved.Add(oldKey)
			}
		}

		if stakingFeature := createdOutput.Output().FeatureSet().Staking(); stakingFeature != nil {
			// staking feature is created or updated - create the diff between the account data and new account
			accountDiff.ValidatorStakeChange = int64(stakingFeature.StakedAmount) - int64(accountData.ValidatorStake)
			accountDiff.StakeEndEpochChange = int64(stakingFeature.EndEpoch) - int64(accountData.StakeEndEpoch)
			accountDiff.FixedCostChange = int64(stakingFeature.FixedCost) - int64(accountData.FixedCost)
		} else if consumedOutput.Output().FeatureSet().Staking() != nil {
			// staking feature was removed from an account
			accountDiff.ValidatorStakeChange = -int64(accountData.ValidatorStake)
			accountDiff.StakeEndEpochChange = -int64(accountData.StakeEndEpoch)
			accountDiff.FixedCostChange = -int64(accountData.FixedCost)
		}
	}

	// case 3. the account was created, fill in the diff with the information of the created output.
	for createdAccountID, createdOutput := range createdAccounts {
		// If it is also consumed, we are in case 2 that was handled above.
		if _, exists := consumedAccounts[createdAccountID]; exists {
			continue
		}

		// We might have had an allotment on this account, and the diff already exists
		accountDiff := getAccountDiff(accountDiffs, createdAccountID)

		// Change and PreviousUpdatedTime are either 0 if we did not have an allotment for this account, or we already
		// have some values from the allotment, so no need to set them explicitly.
		accountDiff.NewOutputID = createdOutput.OutputID()
		accountDiff.PreviousOutputID = iotago.EmptyOutputID
		accountDiff.PreviousExpirySlot = 0

		switch createdOutput.Output().Type() {
		// for account outputs, get block issuer keys from the block issuer feature, and check for staking info.
		case iotago.OutputAccount:
			accountDiff.BlockIssuerKeysAdded = createdOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys
			accountDiff.NewExpirySlot = createdOutput.Output().FeatureSet().BlockIssuer().ExpirySlot
			if stakingFeature := createdOutput.Output().FeatureSet().Staking(); stakingFeature != nil {
				accountDiff.ValidatorStakeChange = int64(stakingFeature.StakedAmount)
				accountDiff.StakeEndEpochChange = int64(stakingFeature.EndEpoch)
				accountDiff.FixedCostChange = int64(stakingFeature.FixedCost)
			}
		// for basic outputs (implicit accounts), get block issuer keys from the address in the unlock conditions.
		case iotago.OutputBasic:
			// If the Output is a Basic Output it can only be here if the address is an ImplicitAccountCreationAddress.
			address, _ := createdOutput.Output().UnlockConditionSet().Address().Address.(*iotago.ImplicitAccountCreationAddress)
			accountDiff.BlockIssuerKeysAdded = iotago.NewBlockIssuerKeys(iotago.Ed25519PublicKeyHashBlockIssuerKeyFromImplicitAccountCreationAddress(address))
			accountDiff.NewExpirySlot = iotago.MaxSlotIndex
		}
	}
}

func (l *Ledger) processCreatedAndConsumedAccountOutputs(stateDiff mempool.StateDiff, accountDiffs map[iotago.AccountID]*model.AccountDiff) (createdAccounts map[iotago.AccountID]*utxoledger.Output, consumedAccounts map[iotago.AccountID]*utxoledger.Output, destroyedAccounts ds.Set[iotago.AccountID], err error) {
	createdAccounts = make(map[iotago.AccountID]*utxoledger.Output)
	consumedAccounts = make(map[iotago.AccountID]*utxoledger.Output)
	destroyedAccounts = ds.NewSet[iotago.AccountID]()

	newAccountDelegation := make(map[iotago.ChainID]*iotago.DelegationOutput)

	stateDiff.CreatedStates().ForEach(func(_ mempool.StateID, output mempool.StateMetadata) bool {
		createdOutput, ok := output.State().(*utxoledger.Output)
		if !ok {
			err = ierrors.Errorf("unexpected state type %T while processing created states", output.State())
			return false
		}

		switch createdOutput.OutputType() {
		case iotago.OutputAccount:
			createdAccount, _ := createdOutput.Output().(*iotago.AccountOutput)

			// if we create an account that doesn't have a block issuer feature or staking, we don't need to track the changes.
			// the VM needs to make sure that no staking feature is created, if there was no block issuer feature.
			if createdAccount.FeatureSet().BlockIssuer() == nil && createdAccount.FeatureSet().Staking() == nil {
				return true
			}

			accountID := createdAccount.AccountID
			if accountID.Empty() {
				accountID = iotago.AccountIDFromOutputID(createdOutput.OutputID())
				l.events.AccountCreated.Trigger(accountID)
			}

			createdAccounts[accountID] = createdOutput
		case iotago.OutputDelegation:
			delegationOutput, _ := createdOutput.Output().(*iotago.DelegationOutput)
			delegationID := delegationOutput.DelegationID
			// Check if the output was newly created or if it was transitioned to delayed claiming.
			// Zeroed Delegation ID => newly created.
			// Non-Zero Delegation ID => delayed claiming transition.
			if delegationID == iotago.EmptyDelegationID() {
				delegationID = iotago.DelegationIDFromOutputID(createdOutput.OutputID())
				newAccountDelegation[delegationID] = delegationOutput
			}

		case iotago.OutputBasic:
			// if a basic output is sent to an implicit account creation address, we need to create the account
			if createdOutput.Output().UnlockConditionSet().Address().Address.Type() == iotago.AddressImplicitAccountCreation {
				accountID := iotago.AccountIDFromOutputID(createdOutput.OutputID())
				l.events.AccountCreated.Trigger(accountID)
				createdAccounts[accountID] = createdOutput
			}
		}

		return true
	})

	if err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "error while processing created states")
	}

	// input side
	stateDiff.DestroyedStates().ForEach(func(_ mempool.StateID, stateMetadata mempool.StateMetadata) bool {
		spentOutput, ok := stateMetadata.State().(*utxoledger.Output)
		if !ok {
			err = ierrors.Errorf("unexpected state type %T while processing destroyed states", stateMetadata.State())
			return false
		}

		switch spentOutput.OutputType() {
		case iotago.OutputAccount:
			consumedAccount, _ := spentOutput.Output().(*iotago.AccountOutput)
			accountID := consumedAccount.AccountID
			if accountID == iotago.EmptyAccountID {
				accountID = iotago.AccountIDFromOutputID(spentOutput.OutputID())
			}

			// if we transition / destroy an account output that doesn't have a block issuer feature or staking, we don't need to track the changes.
			if consumedAccount.FeatureSet().BlockIssuer() == nil && consumedAccount.FeatureSet().Staking() == nil {
				return true
			}
			consumedAccounts[accountID] = spentOutput

			// if we have consumed accounts that are not created in the same slot, we need to track them as destroyed
			if _, exists := createdAccounts[accountID]; !exists {
				destroyedAccounts.Add(accountID)
			}

		case iotago.OutputDelegation:
			delegationOutput, _ := spentOutput.Output().(*iotago.DelegationOutput)
			// The Delegation Output was destroyed or transitioned to delayed claiming.
			// Delegation ID zeroed => Output was in delegating state => remove the stake.
			// Delegation ID non-zero => Output was in delayed claiming state, hence not delegating => no need to remove the stake.
			if delegationOutput.DelegationID.Empty() {
				accountDiff := getAccountDiff(accountDiffs, delegationOutput.ValidatorAddress.AccountID())
				accountDiff.DelegationStakeChange -= int64(delegationOutput.DelegatedAmount)
			}

		case iotago.OutputBasic:
			// if a basic output (implicit account) is consumed, get the accountID as hash of the output ID.
			if spentOutput.Output().UnlockConditionSet().Address().Address.Type() == iotago.AddressImplicitAccountCreation {
				accountID := iotago.AccountIDFromOutputID(spentOutput.OutputID())
				consumedAccounts[accountID] = spentOutput
			}
		}

		return true
	})

	for _, delegationOutput := range newAccountDelegation {
		// the delegation output was newly created and not transitioned/destroyed => add the stake to the validator account
		accountDiff := getAccountDiff(accountDiffs, delegationOutput.ValidatorAddress.AccountID())
		accountDiff.DelegationStakeChange += int64(delegationOutput.DelegatedAmount)
	}

	if err != nil {
		return nil, nil, nil, ierrors.Wrap(err, "error while processing created states")
	}

	return createdAccounts, consumedAccounts, destroyedAccounts, nil
}

func (l *Ledger) processStateDiffTransactions(stateDiff mempool.StateDiff) (spents utxoledger.Spents, outputs utxoledger.Outputs, accountDiffs map[iotago.AccountID]*model.AccountDiff, err error) {
	accountDiffs = make(map[iotago.AccountID]*model.AccountDiff)

	stateDiff.ExecutedTransactions().ForEach(func(txID iotago.TransactionID, txWithMeta mempool.TransactionMetadata) bool {
		tx, ok := txWithMeta.Transaction().(*iotago.Transaction)
		if !ok {
			err = iotago.ErrTxTypeInvalid
			return false
		}

		inputRefs := tx.Inputs()

		// process outputs
		{
			// input side
			for _, inputRef := range lo.Map(inputRefs, mempool.UTXOInputStateRefFromInput) {
				stateWithMetadata, stateError := l.memPool.StateMetadata(inputRef)
				if stateError != nil {
					err = ierrors.Wrapf(stateError, "failed to retrieve outputs of transaction %s", txID)
					return false
				}
				spent := utxoledger.NewSpent(l.outputFromState(stateWithMetadata.State()), txWithMeta.ID(), stateDiff.Slot())
				spents = append(spents, spent)
			}

			// output side
			if err = txWithMeta.Outputs().ForEach(func(stateMetadata mempool.StateMetadata) error {
				typedOutput, ok := stateMetadata.State().(*utxoledger.Output)
				if !ok {
					err = ierrors.Errorf("unexpected state type %T while processing state diff transactions", stateMetadata.State())
					return err
				}

				output := typedOutput.CopyWithBlockIDAndSlotBooked(txWithMeta.EarliestIncludedAttachment(), stateDiff.Slot())
				outputs = append(outputs, output)

				return nil
			}); err != nil {
				return false
			}
		}

		// process allotments
		{
			for _, allotment := range tx.Allotments {
				// in case it didn't exist, allotments won't change the outputID of the Account,
				// so the diff defaults to empty new and previous outputIDs
				accountDiff := getAccountDiff(accountDiffs, allotment.AccountID)

				accountData, exists, accountErr := l.accountsLedger.Account(allotment.AccountID, stateDiff.Slot()-1)
				if accountErr != nil {
					panic(ierrors.Wrapf(accountErr, "error loading account %s in slot %d", allotment.AccountID, stateDiff.Slot()-1))
				}
				// if the account does not exist in our AccountsLedger it means it doesn't have a BIC feature, so
				// we burn this allotment.
				if !exists {
					continue
				}

				accountDiff.BICChange += iotago.BlockIssuanceCredits(allotment.Mana)
				accountDiff.PreviousUpdatedSlot = accountData.Credits.UpdateSlot

				// we are not transitioning the allotted account, so the new and previous expiry slots are the same
				accountDiff.PreviousExpirySlot = accountData.ExpirySlot
				accountDiff.NewExpirySlot = accountData.ExpirySlot

				// we are not transitioning the allotted account, so the new and previous outputIDs are the same
				accountDiff.NewOutputID = accountData.OutputID
				accountDiff.PreviousOutputID = accountData.OutputID
			}
		}

		return true
	})

	return spents, outputs, accountDiffs, nil
}

func (l *Ledger) resolveAccountOutput(accountID iotago.AccountID, slot iotago.SlotIndex) (*utxoledger.Output, error) {
	accountMetadata, exists, err := l.accountsLedger.Account(accountID, slot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "could not get account information for account %s in slot %d", accountID, slot)
	}
	if !exists {
		return nil, ierrors.WithMessagef(mempool.ErrStateNotFound, "account %s does not exist in slot %d", accountID, slot)
	}

	l.utxoLedger.ReadLockLedger()
	defer l.utxoLedger.ReadUnlockLedger()

	isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(accountMetadata.OutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while checking whether account with output id %s is unspent", accountMetadata.OutputID.ToHex())
	}
	if !isUnspent {
		return nil, ierrors.WithMessagef(mempool.ErrStateNotFound, "unspent account with output id %s not found", accountMetadata.OutputID.ToHex())
	}

	accountOutput, err := l.utxoLedger.ReadOutputByOutputIDWithoutLocking(accountMetadata.OutputID)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while retrieving account with output id %s", accountMetadata.OutputID.ToHex())
	}

	return accountOutput, nil
}

func (l *Ledger) resolveState(stateRef mempool.StateReference) *promise.Promise[mempool.State] {
	p := promise.New[mempool.State]()

	l.utxoLedger.ReadLockLedger()
	defer l.utxoLedger.ReadUnlockLedger()

	switch stateRef.Type() {
	case mempool.StateTypeUTXOInput:
		//nolint:forcetypeassert // we can safely assume that this is a UTXOInputStateRef
		utxoInput := stateRef.(mempool.UTXOInputStateRef).Input
		isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(utxoInput.OutputID())
		if err != nil {
			return p.Reject(ierrors.Join(iotago.ErrUTXOInputInvalid, ierrors.Wrapf(err, "error while retrieving output %s", utxoInput.OutputID())))
		}

		if !isUnspent {
			return p.Reject(ierrors.Join(iotago.ErrInputAlreadySpent, ierrors.WithMessagef(mempool.ErrStateNotFound, "unspent output %s not found", utxoInput.OutputID())))
		}

		// possible to cast `stateRef` to more specialized interfaces here, e.g. for DustOutput
		output, err := l.utxoLedger.ReadOutputByOutputIDWithoutLocking(utxoInput.OutputID())
		if err != nil {
			return p.Reject(ierrors.Join(iotago.ErrUTXOInputInvalid, ierrors.WithMessagef(mempool.ErrStateNotFound, "output %s not found", utxoInput.OutputID())))
		}

		return p.Resolve(output)
	case mempool.StateTypeCommitment:
		//nolint:forcetypeassert // we can safely assume that this is a CommitmentInputStateRef
		commitment := stateRef.(mempool.CommitmentInputStateRef).Input
		loadedCommitment, err := l.loadCommitment(commitment.CommitmentID)
		if err != nil {
			return p.Reject(ierrors.Join(iotago.ErrCommitmentInputReferenceInvalid, ierrors.Wrapf(err, "failed to load commitment %s", commitment.CommitmentID)))
		}

		return p.Resolve(mempool.CommitmentInputStateFromCommitment(loadedCommitment))
	default:
		return p.Reject(ierrors.Errorf("unsupported input type %d", stateRef.Type()))
	}
}

func (l *Ledger) blockPreAccepted(block *blocks.Block) {
	if _, isValidationBlock := block.ValidationBlock(); !isValidationBlock {
		return
	}

	voteRank := ledger.NewBlockVoteRank(block.ID(), block.ProtocolBlock().Header.IssuingTime)

	committee, exists := l.sybilProtection.SeatManager().CommitteeInSlot(block.ID().Slot())
	if !exists {
		panic("committee should exist because we pre-accepted the block")
	}

	seat, exists := committee.GetSeat(block.ProtocolBlock().Header.IssuerID)
	if !exists {
		return
	}

	if err := l.spendDAG.CastVotes(vote.NewVote(seat, voteRank), block.SpenderIDs()); err != nil {
		l.errorHandler(ierrors.Wrapf(err, "failed to cast votes for block %s", block.ID()))
	}
}

func (l *Ledger) loadCommitment(inputCommitmentID iotago.CommitmentID) (*iotago.Commitment, error) {
	c, err := l.commitmentLoader(inputCommitmentID.Slot())
	if err != nil {
		return nil, ierrors.Wrap(err, "could not get commitment inputs")
	}
	// The commitment with the specified ID was not found at that index: we are on a different chain.
	if c == nil {
		return nil, ierrors.Errorf("commitment with ID %s not found at index %d: engine on different chain", inputCommitmentID, inputCommitmentID.Slot())
	}
	storedCommitmentID, err := c.Commitment().ID()
	if err != nil {
		return nil, ierrors.Wrap(err, "could not compute commitment ID")
	}
	if storedCommitmentID != inputCommitmentID {
		return nil, ierrors.Errorf("commitment ID of input %s different to stored commitment %s", inputCommitmentID, storedCommitmentID)
	}

	return c.Commitment(), nil
}

func getAccountDiff(accountDiffs map[iotago.AccountID]*model.AccountDiff, accountID iotago.AccountID) *model.AccountDiff {
	accountDiff, exists := accountDiffs[accountID]
	if !exists {
		// initialize the account diff because it didn't exist before
		accountDiff = model.NewAccountDiff()
		accountDiffs[accountID] = accountDiff
	}

	return accountDiff
}
