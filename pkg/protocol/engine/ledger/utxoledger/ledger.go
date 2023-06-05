package utxoledger

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
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/bic"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/mana"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	mempoolv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/v1"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

var ErrUnexpectedUnderlyingType = errors.New("unexpected underlying type provided by the interface")

type Ledger struct {
	apiProviderFunc func() iotago.API

	utxoLedger       *ledgerstate.Manager
	accountsLedger   *bic.BICManager
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
			e.Storage.BICDiffs,
			e.API,
			e.ErrorHandler("ledger"),
			e.Storage.Settings().ProtocolParameters(),
		)

		e.Events.ConflictDAG.LinkTo(l.conflictDAG.Events())

		// TODO: should this attach to RatifiedAccepted instead?
		e.Events.BlockGadget.BlockAccepted.Hook(l.BlockAccepted)

		e.HookConstructed(func() {
			wpBic := ledgerWorkers.CreateGroup("BIC").CreatePool("trackBurnt", 1)
			e.Events.BlockGadget.BlockRatifiedAccepted.Hook(l.accountsLedger.TrackBlock, event.WithWorkerPool(wpBic))
		})
		e.HookInitialized(func() {
			l.accountsLedger.SetBICIndex(e.Storage.Settings().LatestCommitment().Index())
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
	slotDiffFunc func(iotago.SlotIndex) *prunable.BICDiffs,
	apiProviderFunc func() iotago.API,
	errorHandler func(error),
	protocolParameters *iotago.ProtocolParameters,
) *Ledger {
	l := &Ledger{
		apiProviderFunc:    apiProviderFunc,
		utxoLedger:         ledgerstate.New(utxoStore, apiProviderFunc),
		accountsLedger:     bic.New(blocksFunc, slotDiffFunc, accountsStore, apiProviderFunc()),
		commitmentLoader:   commitmentLoader,
		conflictDAG:        conflictdagv1.New[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower](committee),
		errorHandler:       errorHandler,
		protocolParameters: protocolParameters,
	}

	l.memPool = mempoolv1.New(l.executeStardustVM, l.resolveState, workers.CreateGroup("MemPool"), l.conflictDAG, mempoolv1.WithForkAllTransactions[booker.BlockVotePower](true))
	l.manaManager = mana.NewManager(protocolParameters.DecayProvider(), l.resolveAccountOutput)

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

func (l *Ledger) resolveAccountOutput(accountID iotago.AccountID, slotIndex iotago.SlotIndex) (*ledgerstate.Output, error) {
	// make sure that slotIndex is committed
	account, _, err := l.accountsLedger.BIC(accountID, slotIndex)
	if err != nil {
		return nil, xerrors.Errorf("could not get account information for account %s in slot %d: %w", accountID, slotIndex, err)
	}

	l.utxoLedger.ReadLockLedger()
	defer l.utxoLedger.ReadUnlockLedger()

	isUnspent, err := l.utxoLedger.IsOutputIDUnspentWithoutLocking(account.OutputID())
	if err != nil {
		return nil, xerrors.Errorf("error while checking account output %s is unspent: %w", account.OutputID(), err)
	}
	if !isUnspent {
		return nil, xerrors.Errorf("unspent account output %s not found: %w", account.OutputID(), mempool.ErrStateNotFound)
	}

	accountOutput, err := l.utxoLedger.ReadOutputByOutputIDWithoutLocking(account.OutputID())
	if err != nil {
		return nil, xerrors.Errorf("error while retrieving account output %s: %w", account.OutputID(), err)
	}

	return accountOutput, nil
}

func (l *Ledger) resolveState(stateRef iotago.IndexedUTXOReferencer) *promise.Promise[mempool.State] {
	p := promise.New[mempool.State]()

	// possible to cast `stateRef` to more specialized interfaces here, e.g. for DustOutput
	output, err := l.utxoLedger.ReadOutputByOutputID(stateRef.Ref())
	if err != nil {
		p.Reject(xerrors.Errorf("output %s not found: %w", stateRef.Ref(), mempool.ErrStateNotFound))
	} else {
		p.Resolve(output)
	}

	return p
}

func (l *Ledger) Output(stateRef iotago.IndexedUTXOReferencer) (*ledgerstate.Output, error) {
	stateWithMetadata, err := l.memPool.StateMetadata(stateRef)
	if err != nil {
		return nil, err
	}

	switch castState := stateWithMetadata.State().(type) {
	case *ledgerstate.Output:
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

		return ledgerstate.CreateOutput(l.utxoLedger.API(), stateWithMetadata.State().OutputID(), earliestAttachment, earliestAttachment.Index(), tx.Essence.CreationTime, stateWithMetadata.State().Output()), nil
	default:
		panic("unexpected State type")
	}
}

func (l *Ledger) CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, bicRoot iotago.Identifier, err error) {
	ledgerIndex, err := l.utxoLedger.ReadLedgerIndex()
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, err
	}

	if index != ledgerIndex+1 {
		panic(fmt.Errorf("there is a gap in the ledgerstate %d vs %d", ledgerIndex, index))
	}

	stateDiff := l.memPool.StateDiff(index)

	var innerErr error
	var outputs ledgerstate.Outputs
	var spents ledgerstate.Spents
	bicDiffs := make(map[iotago.AccountID]*prunable.BICDiff)
	destroyedAccounts := advancedset.New[iotago.AccountID]()
	consumedAccounts := make(map[iotago.AccountID]*ledgerstate.Output)
	createdAccounts := make(map[iotago.AccountID]*ledgerstate.Output)

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

		// input side
		for _, inputRef := range inputRefs {
			inputState, outputErr := l.Output(inputRef)
			if outputErr != nil {
				innerErr = outputErr
				return false
			}

			spent := ledgerstate.NewSpent(inputState, txWithMeta.ID(), index)
			spents = append(spents, spent)
		}

		// output side
		if createOutputErr := txWithMeta.Outputs().ForEach(func(element mempool.StateMetadata) error {
			output := ledgerstate.CreateOutput(l.utxoLedger.API(), element.State().OutputID(), txWithMeta.EarliestIncludedAttachment(), index, txCreationTime, element.State().Output())
			outputs = append(outputs, output)

			return nil
		}); createOutputErr != nil {
			innerErr = createOutputErr
			return false
		}

		for _, allotment := range tx.Essence.Allotments {
			bicDiff, exists := bicDiffs[allotment.AccountID]
			if !exists {
				bicDiff = prunable.NewBICDiff(l.apiProviderFunc())
				bicDiffs[allotment.AccountID] = bicDiff
			}
			bicDiff.Change += int64(allotment.Value)
			// TODO: what about the PreviousUpdatedTime?
		}

		return true
	})

	if innerErr != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
	}

	// Now we process the account changes, for that we consume the compacted state diff to get the overrall account changes
	// at UTXO level without needing to worry about multiple spends of the same account in the same slot, we only care
	// about the initial account output to be consumed and the final account output to be created.

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

	// process destroyed and transitioned accounts
	for consumedAccountID, consumedAccountOutput := range consumedAccounts {
		// We might have had an allotment on this account, and the diff already exists
		bicDiff, exists := bicDiffs[consumedAccountID]
		if !exists {
			bicDiff = prunable.NewBICDiff(l.apiProviderFunc())
			bicDiffs[consumedAccountID] = bicDiff
		}
		accountData, exists, err := l.accountsLedger.BIC(consumedAccountID, index-1)
		if !exists {
			panic(fmt.Errorf("could not find destroyed account %s in slot %d", consumedAccountID, index-1))
		}
		if err != nil {
			panic(fmt.Errorf("error loading account %s in slot %d: %w", consumedAccountID, index-1, err))
		}

		// the account was destroyed, reverse all the current account data
		if _, exists := createdAccounts[consumedAccountID]; !exists {
			bicDiff.Change -= accountData.Credits().Value
			bicDiff.PreviousUpdatedTime = accountData.Credits().UpdateTime
			bicDiff.NewOutputID = iotago.OutputID{}
			bicDiff.PreviousOutputID = consumedAccountOutput.OutputID()
			bicDiff.PubKeysRemoved = accountData.PubKeys().Slice()
		} else { // the account was transitioned, fill in the diff with the previous information to allow rollback
			// TODO: do not apply any Change, is it correct?
			createdOutput := createdAccounts[consumedAccountID]
			bicDiff.NewOutputID = createdOutput.OutputID()
			bicDiff.PreviousOutputID = consumedAccountOutput.OutputID()

			oldPubKeysSet := accountData.PubKeys()
			newPubKeysSet := advancedset.New[ed25519.PublicKey]()
			for _, pubKey := range createdOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys {
				newPubKeysSet.Add(ed25519.PublicKey(pubKey))
			}

			// Add public keys that are not in the old set
			bicDiff.PubKeysAdded = newPubKeysSet.Filter(func(key ed25519.PublicKey) bool {
				return !oldPubKeysSet.Has(key)
			}).Slice()

			bicDiff.PubKeysRemoved = oldPubKeysSet.Filter(func(key ed25519.PublicKey) bool {
				return !newPubKeysSet.Has(key)
			}).Slice()
		}
	}

	// process created accounts
	for createdAccountID, createdAccountOutput := range createdAccounts {
		// If it is also consumed it is covered by the previous cases
		if _, exists := consumedAccounts[createdAccountID]; exists {
			continue
		}

		// We might have had an allotment on this account, and the diff already exists
		bicDiff, exists := bicDiffs[createdAccountID]
		if !exists {
			bicDiff = prunable.NewBICDiff(l.apiProviderFunc())
			bicDiffs[createdAccountID] = bicDiff
		}

		bicDiff.NewOutputID = createdAccountOutput.OutputID()
		bicDiff.PreviousOutputID = iotago.OutputID{}
		bicDiff.PubKeysAdded = lo.Map(createdAccountOutput.Output().FeatureSet().BlockIssuer().BlockIssuerKeys, func(pk cryptoed25519.PublicKey) ed25519.PublicKey { return ed25519.PublicKey(pk) })

	}

	l.manaManager.CommitSlot(index, destroyedAccounts, createdAccounts)

	if innerErr != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
	}

	if err = l.utxoLedger.ApplyDiff(index, outputs, spents); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, nil
	}
	// TODO: polulate pubKeys, destroyed accounts before commiting
	if bicRoot, err = l.accountsLedger.CommitSlot(index, bicDiffs, destroyedAccounts); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, iotago.Identifier{}, errors.Wrapf(err, "failed to commit slot %d", index)
	}

	// Mark the transactions as committed so the mempool can evict it.
	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, tx mempool.TransactionMetadata) bool {
		tx.Commit()
		return true
	})

	return l.utxoLedger.StateTreeRoot(), iotago.Identifier(stateDiff.Mutations().Root()), bicRoot, nil
}

func (l *Ledger) IsOutputUnspent(outputID iotago.OutputID) (bool, error) {
	return l.utxoLedger.IsOutputIDUnspentWithoutLocking(outputID)
}

func (l *Ledger) Spent(outputID iotago.OutputID) (*ledgerstate.Spent, error) {
	return l.utxoLedger.ReadSpentForOutputIDWithoutLocking(outputID)
}

func (l *Ledger) StateDiffs(index iotago.SlotIndex) (*ledgerstate.SlotDiff, error) {
	return l.utxoLedger.SlotDiffWithoutLocking(index)
}

func (l *Ledger) AddUnspentOutput(unspentOutput *ledgerstate.Output) error {
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
