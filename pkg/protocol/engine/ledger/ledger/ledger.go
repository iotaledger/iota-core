package ledger

import (
	cryptoed25519 "crypto/ed25519"
	"fmt"
	"io"

	"golang.org/x/xerrors"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/accountsledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/mana"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
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

	memPool            mempool.MemPool[booker.BlockVotePower]
	conflictDAG        conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]
	errorHandler       func(error)
	protocolParameters *iotago.ProtocolParameters

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

		// TODO: should this attach to RatifiedAccepted instead?
		e.Events.BlockGadget.BlockAccepted.Hook(l.BlockAccepted)
		e.EvictionState.Events.SlotEvicted.Hook(l.memPool.Evict)
		// TODO: when should ledgerState be pruned?

		e.HookConstructed(func() {
			wpAccounts := ledgerWorkers.CreateGroup("Accounts").CreatePool("trackBurnt", 1)
			e.Events.BlockGadget.BlockRatifiedAccepted.Hook(l.accountsLedger.TrackBlock, event.WithWorkerPool(wpAccounts))
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
		conflictDAG:        conflictdagv1.New[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower](committee),
		errorHandler:       errorHandler,
		protocolParameters: protocolParameters,
	}

	l.memPool = mempoolv1.New(l.executeStardustVM, l.resolveState, workers.CreateGroup("MemPool"), l.conflictDAG, mempoolv1.WithForkAllTransactions[booker.BlockVotePower](true))

	// TODO: We wanted to avoid having logic in `ProtocolParams` struct, so we created a separate DecayProvider.
	// DecayProvider is created here once and once during each transaction execution.
	// Is it better to have a single instance that is passed to the VM from the outside through vmParams? Or something else?
	// `protocolParameters` are always passed to the VM, and DecayProvider can be derived from that.
	l.manaManager = mana.NewManager(l.protocolParameters.DecayProvider(), l.resolveAccountOutput)

	return l
}

func (l *Ledger) ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower] {
	return l.conflictDAG
}

func (l *Ledger) Shutdown() {
	l.TriggerStopped()
	l.conflictDAG.Shutdown()
}

func (l *Ledger) Import(reader io.ReadSeeker) error {
	if err := l.utxoLedger.Import(reader); err != nil {
		return err
	}

	if err := l.accountsLedger.Import(reader); err != nil {
		return err
	}

	return nil
}

func (l *Ledger) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	if err := l.utxoLedger.Export(writer, targetIndex); err != nil {
		return err
	}

	if err := l.accountsLedger.Export(writer, targetIndex); err != nil {
		return err
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

	isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(accountMetadata.OutputID())
	if err != nil {
		return nil, xerrors.Errorf("error while checking account output %s is unspent: %w", accountMetadata.OutputID(), err)
	}
	if !isUnspent {
		return nil, xerrors.Errorf("unspent account output %s not found: %w", accountMetadata.OutputID(), mempool.ErrStateNotFound)
	}

	accountOutput, err := l.utxoLedger.ReadOutputByOutputIDWithoutLocking(accountMetadata.OutputID())
	if err != nil {
		return nil, xerrors.Errorf("error while retrieving account output %s: %w", accountMetadata.OutputID(), err)
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
				if createOutputErr := txWithMeta.Outputs().ForEach(func(element mempool.StateMetadata) error {
					output := utxoledger.CreateOutput(l.utxoLedger.API(), element.State().OutputID(), txWithMeta.EarliestIncludedAttachment(), index, txCreationTime, element.State().Output())
					outputs = append(outputs, output)

					return nil
				}); createOutputErr != nil {
					innerErr = createOutputErr
					return false
				}
			}

			// process allotments
			{
				// TODO: who checks that the allotment goes to an Account with a BIC feature?
				for _, allotment := range tx.Essence.Allotments {
					accountDiff, exists := accountDiffs[allotment.AccountID]
					if !exists {
						// allotments won't change the outputID of the Account, so the diff defaults to empty new and previous outputIDs
						accountDiff = prunable.NewAccountDiff(l.apiProvider())
						accountDiffs[allotment.AccountID] = accountDiff
					}

					accountData, exists, err := l.accountsLedger.Account(allotment.AccountID)
					if !exists {
						panic(fmt.Errorf("could not find destroyed account %s in slot %d", allotment.AccountID, index-1))
					}
					if err != nil {
						panic(fmt.Errorf("error loading account %s in slot %d: %w", allotment.AccountID, index-1, err))
					}

					accountDiff.Change += int64(allotment.Value)
					accountDiff.PreviousUpdatedTime = accountData.BlockIssuanceCredits().UpdateTime

					// we are not transitioning the allotted account, so the new and previous outputIDs are the same
					accountDiff.NewOutputID = accountData.OutputID()
					accountDiff.PreviousOutputID = accountData.OutputID()
				}
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
		}
	}

	// Now we process the collect account changes, for that we consume the "compacted" state diff to get the overrall
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
				createdAccount := createdOutput.Output().(*iotago.AccountOutput)
				createdAccounts[createdAccount.AccountID] = createdOutput
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
		}

		// input side
		stateDiff.DestroyedStates().ForEachKey(func(outputID iotago.OutputID) bool {
			spentOutput, err := l.Output(outputID.UTXOInput())
			if err != nil {
				innerErr = err
				return false
			}

			if spentOutput.OutputType() == iotago.OutputAccount {
				consumedAccount := spentOutput.Output().(*iotago.AccountOutput)
				consumedAccounts[consumedAccount.AccountID] = spentOutput

				// if we have consumed accounts that are not created in the same slot, we need to track them as destroyed
				if _, exists := createdAccounts[consumedAccount.AccountID]; !exists {
					destroyedAccounts.Add(consumedAccount.AccountID)
				}
			}

			return true
		})

		if innerErr != nil {
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
		}
	}

	// process the collected account changes.
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
				accountDiff = prunable.NewAccountDiff(l.apiProvider())
				accountDiffs[consumedAccountID] = accountDiff
			}

			accountData, exists, err := l.accountsLedger.Account(consumedAccountID, index-1)
			if err != nil {
				panic(fmt.Errorf("error loading account %s in slot %d: %w", consumedAccountID, index-1, err))
			}
			if !exists {
				panic(fmt.Errorf("could not find destroyed account %s in slot %d", consumedAccountID, index-1))
			}

			// case 1. the account was destroyed, the diff is the reverse of all current account's data
			{
				if destroyedAccounts.Has(consumedAccountID) {
					accountDiff.Change = -accountData.BlockIssuanceCredits().Value
					accountDiff.PreviousUpdatedTime = accountData.BlockIssuanceCredits().UpdateTime
					accountDiff.NewOutputID = iotago.OutputID{}
					accountDiff.PreviousOutputID = accountData.OutputID()
					accountDiff.PubKeysRemoved = accountData.PubKeys().Slice()

					continue
				}
			}

			// case 2. the account was transitioned, fill in the diff with the delta information
			// Change and PreviousUpdatedTime are either 0 if we did not have an allotment for this account, or we already
			// have some values from the allotment, so no need to set them explicitly.
			{
				createdOutput := createdAccounts[consumedAccountID]
				accountDiff.NewOutputID = createdOutput.OutputID()
				accountDiff.PreviousOutputID = consumedOutput.OutputID()

				oldPubKeysSet := accountData.PubKeys()
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
					accountDiff = prunable.NewAccountDiff(l.apiProvider())
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
			return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
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

func (l *Ledger) AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool) {
	switch payload := block.Block().Payload.(type) {
	case mempool.Transaction:
		transactioMetadata, err := l.memPool.AttachTransaction(payload, block.ID())
		if err != nil {
			l.errorHandler(err)

			return nil, true
		}

		return transactioMetadata, true
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

func (l *Ledger) IsAccountLocked(block *iotago.Block) bool {
	blockSlotIndex := l.protocolParameters.SlotTimeProvider().IndexFromTime(block.IssuingTime)
	bicSlot := blockSlotIndex - iotago.SlotIndex(l.protocolParameters.MaxCommitableAge)
	account, exists, err := l.accountsLedger.Account(block.IssuerID, bicSlot)
	if err != nil {
		return true
	}
	if !exists {
		return true
	}
	if account.BlockIssuanceCredits().Value < 0 {
		return true
	}
	return false
}
