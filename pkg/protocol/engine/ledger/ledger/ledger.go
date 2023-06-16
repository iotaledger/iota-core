package ledger

import (
	cryptoed25519 "crypto/ed25519"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/mana"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	mempoolv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

var ErrUnexpectedUnderlyingType = errors.New("unexpected underlying type provided by the interface")

type Ledger struct {
	apiProvider func() iotago.API

	utxoLedger       *utxoledger.Manager
	accountsLedger   *accountsledger.Manager
	manaManager      *mana.Manager
	commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error)

	memPool            mempool.MemPool[ledger.BlockVotePower]
	conflictDAG        conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower]
	errorHandler       func(error)
	protocolParameters *iotago.ProtocolParameters

	decayProvider *iotago.ManaDecayProvider

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, ledger.Ledger] {
	return module.Provide(func(e *engine.Engine) ledger.Ledger {
		ledgerWorkers := e.Workers.CreateGroup("Ledger")

		l := New(
			ledgerWorkers,
			e.Storage.Ledger(),
			e.Storage.Accounts(),
			e.Storage.Commitments().Load,
			e.SybilProtection.OnlineCommittee(),
			e.BlockCache.Block,
			e.Storage.AccountDiffs,
			e.API,
			e.ErrorHandler("ledger"),
			e.Storage.Settings().ProtocolParameters(),
		)

		e.Events.ConflictDAG.LinkTo(l.conflictDAG.Events())

		e.Events.BlockGadget.BlockPreAccepted.Hook(l.blockPreAccepted)

		e.Events.BlockGadget.BlockAccepted.Hook(l.BlockAccepted)
		e.EvictionState.Events.SlotEvicted.Hook(l.memPool.Evict)
		// TODO: when should ledgerState be pruned?

		e.HookConstructed(func() {
			wpAccounts := ledgerWorkers.CreateGroup("Accounts").CreatePool("trackBurnt", 1)
			e.Events.BlockGadget.BlockAccepted.Hook(l.accountsLedger.TrackBlock, event.WithWorkerPool(wpAccounts))
		})
		e.HookInitialized(func() {
			l.accountsLedger.SetLatestCommittedSlot(e.Storage.Settings().LatestCommitment().Index())
		})

		return l
	})
}

func New(
	workers *workerpool.Group,
	utxoStore kvstore.KVStore,
	accountsStore kvstore.KVStore,
	commitmentLoader func(iotago.SlotIndex) (*model.Commitment, error),
	committee *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID],
	blocksFunc func(id iotago.BlockID) (*blocks.Block, bool),
	slotDiffFunc func(iotago.SlotIndex) *prunable.AccountDiffs,
	apiProvider func() iotago.API,
	errorHandler func(error),
	protocolParameters *iotago.ProtocolParameters,
) *Ledger {
	l := &Ledger{
		apiProvider:        apiProvider,
		utxoLedger:         utxoledger.New(utxoStore, apiProvider),
		accountsLedger:     accountsledger.New(blocksFunc, slotDiffFunc, accountsStore, apiProvider(), protocolParameters.MaxCommitableAge),
		commitmentLoader:   commitmentLoader,
		conflictDAG:        conflictdagv1.New[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower](committee),
		errorHandler:       errorHandler,
		decayProvider:      protocolParameters.DecayProvider(),
		protocolParameters: protocolParameters,
	}

	l.memPool = mempoolv1.New(l.executeStardustVM, l.resolveState, workers.CreateGroup("MemPool"), l.conflictDAG, mempoolv1.WithForkAllTransactions[ledger.BlockVotePower](true))

	// TODO: We wanted to avoid having logic in `ProtocolParams` struct, so we created a separate DecayProvider.
	// DecayProvider is created here once and once during each transaction execution.
	// Is it better to have a single instance that is passed to the VM from the outside through vmParams? Or something else?
	// `protocolParameters` are always passed to the VM, and DecayProvider can be derived from that.
	l.manaManager = mana.NewManager(l.protocolParameters.DecayProvider(), l.resolveAccountOutput)

	return l
}

func (l *Ledger) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower] {
	return l.conflictDAG
}

func (l *Ledger) Shutdown() {
	l.TriggerStopped()
	l.conflictDAG.Shutdown()
}

func (l *Ledger) Import(reader io.ReadSeeker) error {
	if err := l.utxoLedger.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import utxoLedger")
	}

	if err := l.accountsLedger.Import(reader); err != nil {
		return errors.Wrap(err, "failed to import accountsLedger")
	}

	return nil
}

func (l *Ledger) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	if err := l.utxoLedger.Export(writer, targetIndex); err != nil {
		return errors.Wrap(err, "failed to export utxoLedger")
	}

	if err := l.accountsLedger.Export(writer, targetIndex); err != nil {
		return errors.Wrap(err, "failed to export accountsLedger")
	}

	return nil
}

func (l *Ledger) resolveAccountOutput(accountID iotago.AccountID, slotIndex iotago.SlotIndex) (*utxoledger.Output, error) {
	accountMetadata, _, err := l.accountsLedger.Account(accountID, slotIndex)
	if err != nil {
		return nil, xerrors.Errorf("could not get account information for account %s in slot %d: %w", accountID, slotIndex, err)
	}

	l.utxoLedger.ReadLockLedger()
	defer l.utxoLedger.ReadUnlockLedger()

	isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(accountMetadata.OutputID)
	if err != nil {
		return nil, xerrors.Errorf("error while checking account output %s is unspent: %w", accountMetadata.OutputID, err)
	}
	if !isUnspent {
		return nil, xerrors.Errorf("unspent account output %s not found: %w", accountMetadata.OutputID, mempool.ErrStateNotFound)
	}

	accountOutput, err := l.utxoLedger.ReadOutputByOutputIDWithoutLocking(accountMetadata.OutputID)
	if err != nil {
		return nil, xerrors.Errorf("error while retrieving account output %s: %w", accountMetadata.OutputID, err)
	}

	return accountOutput, nil
}

func (l *Ledger) resolveState(stateRef iotago.IndexedUTXOReferencer) *promise.Promise[mempool.State] {
	p := promise.New[mempool.State]()

	l.utxoLedger.ReadLockLedger()
	defer l.utxoLedger.ReadUnlockLedger()

	isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(stateRef.Ref())
	if err != nil {
		p.Reject(xerrors.Errorf("error while retrieving output %s: %w", stateRef.Ref(), err))
	}

	if !isUnspent {
		p.Reject(xerrors.Errorf("unspent output %s not found: %w", stateRef.Ref(), mempool.ErrStateNotFound))
	}

	// possible to cast `stateRef` to more specialized interfaces here, e.g. for DustOutput
	output, err := l.utxoLedger.ReadOutputByOutputID(stateRef.Ref())

	if err != nil {
		p.Reject(xerrors.Errorf("output %s not found: %w", stateRef.Ref(), mempool.ErrStateNotFound))
	} else {
		p.Resolve(output)
	}

	return p
}

func (l *Ledger) Output(stateRef iotago.IndexedUTXOReferencer) (*utxoledger.Output, error) {
	stateWithMetadata, err := l.memPool.StateMetadata(stateRef)
	if err != nil {
		return nil, err
	}

	switch castState := stateWithMetadata.State().(type) {
	case *utxoledger.Output:
		return castState, nil
	case *ExecutionOutput:
		txWithMetadata, exists := l.memPool.TransactionMetadata(stateRef.Ref().TransactionID())
		if !exists {
			return nil, err
		}

		earliestAttachment := txWithMetadata.EarliestIncludedAttachment()

		tx, ok := txWithMetadata.Transaction().(*iotago.Transaction)
		if !ok {
			return nil, ErrUnexpectedUnderlyingType
		}

		return utxoledger.CreateOutput(l.utxoLedger.API(), stateWithMetadata.State().OutputID(), earliestAttachment, earliestAttachment.Index(), tx.Essence.CreationTime, stateWithMetadata.State().Output()), nil
	default:
		panic("unexpected State type")
	}
}

func (l *Ledger) Account(accountID iotago.AccountID, optTargetIndex ...iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error) {
	return l.accountsLedger.Account(accountID, optTargetIndex...)
}

func (l *Ledger) CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, accountRoot iotago.Identifier, err error) {
	ledgerIndex, err := l.utxoLedger.ReadLedgerIndex()
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, err
	}

	if index != ledgerIndex+1 {
		panic(fmt.Errorf("there is a gap in the ledgerstate %d vs %d", ledgerIndex, index))
	}

	stateDiff := l.memPool.StateDiff(index)

	var innerErr error
	var outputs utxoledger.Outputs
	var spents utxoledger.Spents
	accountDiffs := make(map[iotago.AccountID]*prunable.AccountDiff)
	destroyedAccounts := advancedset.New[iotago.AccountID]()
	consumedAccounts := make(map[iotago.AccountID]*utxoledger.Output)
	createdAccounts := make(map[iotago.AccountID]*utxoledger.Output)

	// collect outputs and allotments from the "uncompacted" statediff
	// outputs need to be processed in the "uncompacted" version of the state diff, as we need to be able to store
	// and retrieve intermediate outputs to show to the user
	{
		stateDiff.ExecutedTransactions().ForEach(func(txID iotago.TransactionID, txWithMeta mempool.TransactionMetadata) bool {
			tx, ok := txWithMeta.Transaction().(*iotago.Transaction)
			if !ok {
				innerErr = ErrUnexpectedUnderlyingType
				return false
			}
			txCreationTime := tx.Essence.CreationTime

			inputRefs, errInput := tx.Inputs()
			if errInput != nil {
				innerErr = errInput
				return false
			}

			// process outputs
			{
				// input side
				for _, inputRef := range inputRefs {
					inputState, outputErr := l.Output(inputRef)
					if outputErr != nil {
						innerErr = outputErr
						return false
					}

					spent := utxoledger.NewSpent(inputState, txWithMeta.ID(), index)
					spents = append(spents, spent)
				}

				// output side
				if createOutputErr := txWithMeta.Outputs().ForEach(func(stateMetadata mempool.StateMetadata) error {
					output := utxoledger.CreateOutput(l.utxoLedger.API(), stateMetadata.State().OutputID(), txWithMeta.EarliestIncludedAttachment(), index, txCreationTime, stateMetadata.State().Output())
					outputs = append(outputs, output)

					return nil
				}); createOutputErr != nil {
					innerErr = createOutputErr
					return false
				}
			}

			// process allotments
			{
				for _, allotment := range tx.Essence.Allotments {
					// TODO: what happens if mana is allotted to non-existent account? Do we even get to this point?
					accountDiff, exists := accountDiffs[allotment.AccountID]
					if !exists {
						// allotments won't change the outputID of the Account, so the diff defaults to empty new and previous outputIDs
						accountDiff = prunable.NewAccountDiff()
						accountDiffs[allotment.AccountID] = accountDiff
					}
					accountData, exists, err := l.accountsLedger.Account(allotment.AccountID)
					if err != nil {
						panic(fmt.Errorf("error loading account %s in slot %d: %w", allotment.AccountID, index-1, err))
					}
					// if the account does not exist in our AccountsLedger it means it doesn't have a BIC feature, so
					// we burn this allotment.
					if !exists {
						continue
					}

					accountDiff.Change += int64(allotment.Value)
					accountDiff.PreviousUpdatedTime = accountData.Credits.UpdateTime

					// we are not transitioning the allotted account, so the new and previous outputIDs are the same
					accountDiff.NewOutputID = accountData.OutputID
					accountDiff.PreviousOutputID = accountData.OutputID
				}
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(innerErr, "failed to commit slot %d", index)
		}
	}

	// Now we process the collect account changes, for that we consume the "compacted" state diff to get the overall
	// account changes at UTXO level without needing to worry about multiple spends of the same account in the same slot,
	// we only care about the initial account output to be consumed and the final account output to be created.
	{
		// output side
		stateDiff.CreatedStates().ForEachKey(func(outputID iotago.OutputID) bool {
			createdOutput, err := l.Output(outputID.UTXOInput())
			if err != nil {
				innerErr = err
				return false
			}

			if createdOutput.OutputType() == iotago.OutputAccount {
				createdAccount, _ := createdOutput.Output().(*iotago.AccountOutput)

				// Skip if the account doesn't have a BIC feature.
				if createdAccount.FeatureSet().BlockIssuer() == nil {
					return true
				}

				accountID := createdAccount.AccountID
				if accountID.Empty() {
					accountID = iotago.AccountIDFromOutputID(outputID)
				}

				createdAccounts[accountID] = createdOutput
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(innerErr, "failed to commit slot %d", index)
		}

		// input side
		stateDiff.DestroyedStates().ForEachKey(func(outputID iotago.OutputID) bool {
			spentOutput, err := l.Output(outputID.UTXOInput())
			if err != nil {
				innerErr = err
				return false
			}

			if spentOutput.OutputType() == iotago.OutputAccount {
				consumedAccount, _ := spentOutput.Output().(*iotago.AccountOutput)
				// Skip if the account doesn't have a BIC feature.
				if consumedAccount.FeatureSet().BlockIssuer() == nil {
					return true
				}
				consumedAccounts[consumedAccount.AccountID] = spentOutput

				// if we have consumed accounts that are not created in the same slot, we need to track them as destroyed
				if _, exists := createdAccounts[consumedAccount.AccountID]; !exists {
					destroyedAccounts.Add(consumedAccount.AccountID)
				}
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(innerErr, "failed to commit slot %d", index)
		}
	}

	// process the collected account changes. The consumedAccounts and createdAccounts maps only contain outputs with a
	// BIC feature, so allotments made to accounts without a BIC feature are not tracked here and they are burned as a result.
	// There are 3 possible cases:
	// 1. The account was only consumed but not created in this slot, therefore it is marked as destroyed and its latest
	// state is stored as diff to allow a rollback.
	// 2. The account was consumed and created in the same slot, the account was transitioned, and we have to store the
	// changes in the diff.
	// 3. The account was only created in this slot, in this case we need to track the output's values as the diff.
	{
		for consumedAccountID, consumedOutput := range consumedAccounts {
			// We might have had an allotment on this account, and the diff already exists
			accountDiff, exists := accountDiffs[consumedAccountID]
			if !exists {
				accountDiff = prunable.NewAccountDiff()
				accountDiffs[consumedAccountID] = accountDiff
			}

			accountData, exists, err := l.accountsLedger.Account(consumedAccountID, index-1)
			if err != nil {
				panic(fmt.Errorf("error loading account %s in slot %d: %w", consumedAccountID, index-1, err))
			}
			if !exists {
				panic(fmt.Errorf("could not find destroyed account %s in slot %d", consumedAccountID, index-1))
			}

			// case 1. the account was destroyed, the diff will be created inside accountLedger on account deletion

			// case 2. the account was transitioned, fill in the diff with the delta information
			// Change and PreviousUpdatedTime are either 0 if we did not have an allotment for this account, or we already
			// have some values from the allotment, so no need to set them explicitly.
			{
				createdOutput := createdAccounts[consumedAccountID]
				accountDiff.NewOutputID = createdOutput.OutputID()
				accountDiff.PreviousOutputID = consumedOutput.OutputID()

				oldPubKeysSet := accountData.PubKeys
				newPubKeysSet := advancedset.New[ed25519.PublicKey]()
				for _, pubKey := range createdOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys {
					newPubKeysSet.Add(ed25519.PublicKey(pubKey))
				}

				// Add public keys that are not in the old set
				accountDiff.PubKeysAdded = newPubKeysSet.Filter(func(key ed25519.PublicKey) bool {
					return !oldPubKeysSet.Has(key)
				}).Slice()

				// Remove the keys that are not in the new set
				accountDiff.PubKeysRemoved = oldPubKeysSet.Filter(func(key ed25519.PublicKey) bool {
					return !newPubKeysSet.Has(key)
				}).Slice()
			}
		}

		// case 3. the account was created, fill in the diff with the information of the created output.
		{
			for createdAccountID, createdOutput := range createdAccounts {
				// If it is also consumed we are in case 2.
				if _, exists := consumedAccounts[createdAccountID]; exists {
					continue
				}

				// We might have had an allotment on this account, and the diff already exists
				accountDiff, exists := accountDiffs[createdAccountID]
				if !exists {
					accountDiff = prunable.NewAccountDiff()
					accountDiffs[createdAccountID] = accountDiff
				}
				// Change and PreviousUpdatedTime are either 0 if we did not have an allotment for this account, or we already
				// have some values from the allotment, so no need to set them explicitly.
				accountDiff.NewOutputID = createdOutput.OutputID()
				accountDiff.PreviousOutputID = iotago.OutputID{}
				accountDiff.PubKeysAdded = lo.Map(createdOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys, func(pk cryptoed25519.PublicKey) ed25519.PublicKey { return ed25519.PublicKey(pk) })
			}
		}
	}

	// Commit the changes
	{
		// Update the mana manager's cache
		l.manaManager.ApplyDiff(index, destroyedAccounts, createdAccounts)

		// Update the UTXO ledger
		if err = l.utxoLedger.ApplyDiff(index, outputs, spents); err != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(err, "failed to commit slot %d", index)
		}

		// Update the Accounts ledger
		if err = l.accountsLedger.ApplyDiff(index, accountDiffs, destroyedAccounts); err != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(err, "failed to commit slot %d", index)
		}

		// Mark each transaction as committed so the mempool can evict it
		stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, tx mempool.TransactionMetadata) bool {
			tx.Commit()
			return true
		})
	}

	return l.utxoLedger.StateTreeRoot(), iotago.Identifier(stateDiff.Mutations().Root()), l.accountsLedger.AccountsTreeRoot(), nil
}

func (l *Ledger) IsOutputUnspent(outputID iotago.OutputID) (bool, error) {
	return l.utxoLedger.IsOutputIDUnspentWithoutLocking(outputID)
}

func (l *Ledger) Spent(outputID iotago.OutputID) (*utxoledger.Spent, error) {
	return l.utxoLedger.ReadSpentForOutputIDWithoutLocking(outputID)
}

func (l *Ledger) StateDiffs(index iotago.SlotIndex) (*utxoledger.SlotDiff, error) {
	return l.utxoLedger.SlotDiffWithoutLocking(index)
}

func (l *Ledger) AddUnspentOutput(unspentOutput *utxoledger.Output) error {
	return l.utxoLedger.AddUnspentOutput(unspentOutput)
}

func (l *Ledger) AddAccount(output *utxoledger.Output) error {
	return l.accountsLedger.AddAccount(output)
}

func (l *Ledger) AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool) {
	switch payload := block.Block().Payload.(type) {
	case mempool.Transaction:
		transactionMetadata, err := l.memPool.AttachTransaction(payload, block.ID())
		if err != nil {
			l.errorHandler(err)

			return nil, true
		}

		return transactionMetadata, true
	default:

		return nil, false
	}
}

func (l *Ledger) OnTransactionAttached(handler func(transaction mempool.TransactionMetadata), opts ...event.Option) {
	l.memPool.OnTransactionAttached(handler, opts...)
}

func (l *Ledger) TransactionMetadata(transactionID iotago.TransactionID) (mempool.TransactionMetadata, bool) {
	return l.memPool.TransactionMetadata(transactionID)
}

func (l *Ledger) TransactionMetadataByAttachment(blockID iotago.BlockID) (mempool.TransactionMetadata, bool) {
	return l.memPool.TransactionMetadataByAttachment(blockID)
}

func (l *Ledger) BlockAccepted(block *blocks.Block) {
	switch block.Block().Payload.(type) {
	case *iotago.Transaction:
		l.memPool.MarkAttachmentIncluded(block.ID())

	default:
		return
	}
}

func (l *Ledger) blockPreAccepted(block *blocks.Block) {
	votePower := ledger.NewBlockVotePower(block.ID(), block.Block().IssuingTime)
	if err := l.conflictDAG.CastVotes(vote.NewVote(block.Block().IssuerID, votePower), block.ConflictIDs()); err != nil {
		// TODO: here we need to check what kind of error and potentially mark the block as invalid.
		//  Do we track witness weight of invalid blocks?
		l.errorHandler(errors.Wrapf(err, "failed to cast votes for block %s", block.ID()))
	}
}
