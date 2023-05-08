package utxoledger

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/core/vote"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	mempoolv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/therealledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger struct {
	ledgerState  *ledgerstate.Manager
	memPool      mempool.MemPool[vote.MockedPower]
	errorHandler func(error)

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, therealledger.Ledger] {
	return module.Provide(func(e *engine.Engine) therealledger.Ledger {
		l := New(e.Workers.CreateGroup("Ledger"), e.Storage.Ledger(), e.API, e.Events.Error.Trigger)
		e.Events.Booker.BlockBooked.Hook(l.attachTransaction)
		e.Events.BlockGadget.BlockAccepted.Hook(l.blockAccepted)

		return l
	})
}

func New(workers *workerpool.Group, store kvstore.KVStore, apiProviderFunc func() iotago.API, errorHandler func(error)) *Ledger {
	l := &Ledger{
		ledgerState:  ledgerstate.New(store, apiProviderFunc),
		errorHandler: errorHandler,
	}

	l.memPool = mempoolv1.New[vote.MockedPower](l.executeStardustVM, l.resolveState, workers.CreateGroup("MemPool"))

	return l
}

func (l *Ledger) Shutdown() {
	l.TriggerStopped()
	//TODO:
	//l.memPool.Shutdown()
}

func (l *Ledger) resolveState(stateRef ledger.StateReference) *promise.Promise[ledger.State] {
	p := promise.New[ledger.State]()

	output, err := l.ledgerState.ReadOutputByOutputID(stateRef.StateID())
	if err != nil {
		p.Reject(xerrors.Errorf("output %s not found: %w", stateRef.StateID(), ledger.ErrStateNotFound))
	} else {
		p.Resolve(&State{
			outputID: output.OutputID(),
			output:   output.Output(),
		})
	}

	return p
}

func (l *Ledger) Output(id iotago.OutputID) (*ledgerstate.Output, error) {
	stateWithMetadata, err := l.memPool.State(ledger.StoredStateReference(id))
	if err != nil {
		return l.ledgerState.ReadOutputByOutputID(id)
	}

	txWithMetadata, exists := l.memPool.Transaction(id.TransactionID())
	if !exists {
		return l.ledgerState.ReadOutputByOutputID(id)
	}

	earliestAttachment := txWithMetadata.EarliestIncludedAttachment()
	state := stateWithMetadata.State().(*State)
	txCreationTime := txWithMetadata.Transaction().(*Transaction).Transaction.Essence.CreationTime

	return ledgerstate.CreateOutput(l.ledgerState.API(), state.outputID, earliestAttachment, earliestAttachment.Index(), txCreationTime, state.output), nil
}

func (l *Ledger) CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, err error) {
	stateDiff, err := l.memPool.StateDiff(index)
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, err
	}

	ledgerIndex, err := l.ledgerState.ReadLedgerIndex()
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, err
	}

	if index != ledgerIndex+1 {
		panic(fmt.Errorf("there is a gap in the ledgerstate %d vs %d", ledgerIndex, index))
	}

	var outputs ledgerstate.Outputs
	var spents ledgerstate.Spents

	for it := stateDiff.Transactions.Iterator(); it.HasNext(); {
		txWithMeta := it.Next()

		tx := txWithMeta.Transaction().(*Transaction)
		txCreationTime := tx.Transaction.Essence.CreationTime

		inputs, err := tx.Inputs()
		if err != nil {
			return iotago.Identifier{}, iotago.Identifier{}, err
		}
		for _, input := range inputs {
			inputOutput, err := l.Output(input.StateID())
			if err != nil {
				return iotago.Identifier{}, iotago.Identifier{}, err
			}

			spent := ledgerstate.NewSpent(inputOutput, txWithMeta.ID(), txCreationTime, index)
			spents = append(spents, spent)
		}

		if err := txWithMeta.Outputs().ForEach(func(element mempool.StateWithMetadata) error {
			state := element.State().(*State)
			output := ledgerstate.CreateOutput(l.ledgerState.API(), state.outputID, iotago.EmptyBlockID(), index, txCreationTime, state.output)
			outputs = append(outputs, output)
			return nil
		}); err != nil {
			return iotago.Identifier{}, iotago.Identifier{}, err
		}
	}

	if err := l.ledgerState.ApplyConfirmation(index, outputs, spents); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, err
	}

	// Mark the transactions as committed so the mempool can evict it.
	for it := stateDiff.Transactions.Iterator(); it.HasNext(); {
		it.Next().SetCommitted()
	}

	//TODO: add missing State tree
	return iotago.Identifier{}, iotago.Identifier(stateDiff.StateMutation.Root()), nil
}

func (l *Ledger) attachTransaction(block *blocks.Block) {
	switch payload := block.Block().Payload.(type) {
	case *iotago.Transaction:
		tx := &Transaction{payload}
		if _, err := l.memPool.AttachTransaction(tx, block.ID()); err != nil {
			l.errorHandler(err)
		}

	default:
		return
	}
}

func (l *Ledger) blockAccepted(block *blocks.Block) {
	switch block.Block().Payload.(type) {
	case *iotago.Transaction:
		if err := l.memPool.MarkAttachmentIncluded(block.ID()); err != nil {
			l.errorHandler(err)
		}

	default:
		return
	}
}
