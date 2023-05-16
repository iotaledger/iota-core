package utxoledger

import (
	"fmt"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledgerstate"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag/conflictdagv1"
	mempoolv1 "github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/v1"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/therealledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

var ErrUnexpectedUnderlyingType = errors.New("unexpected underlying type provided by the interface")

type Ledger struct {
	ledgerState  *ledgerstate.Manager
	memPool      mempool.MemPool[booker.BlockVotePower]
	conflictDAG  conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]
	errorHandler func(error)

	module.Module
}

func NewProvider() module.Provider[*engine.Engine, therealledger.Ledger] {
	return module.Provide(func(e *engine.Engine) therealledger.Ledger {
		l := New(e.Workers.CreateGroup("Ledger"), e.Storage.Ledger(), e.API, e.SybilProtection, e.Events.Error.Trigger)

		// TODO: should this attach to RatifiedAccepted instead?
		e.Events.BlockGadget.BlockAccepted.Hook(l.blockAccepted)

		return l
	})
}

func New(workers *workerpool.Group, store kvstore.KVStore, apiProviderFunc func() iotago.API, sybilProtection sybilprotection.SybilProtection, errorHandler func(error)) *Ledger {
	l := &Ledger{
		ledgerState:  ledgerstate.New(store, apiProviderFunc),
		conflictDAG:  conflictdagv1.New[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower](sybilProtection.OnlineCommittee()),
		errorHandler: errorHandler,
	}

	l.memPool = mempoolv1.New(l.executeStardustVM, l.resolveState, workers.CreateGroup("MemPool"), l.conflictDAG)

	return l
}

func (l *Ledger) Shutdown() {
	l.TriggerStopped()
	l.conflictDAG.Shutdown()
}

func (l *Ledger) Import(reader io.ReadSeeker) error {
	return l.ledgerState.Import(reader)
}

func (l *Ledger) Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error {
	return l.ledgerState.Export(writer, targetIndex)
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
	// TODO: remove loading from ledgerstate as the mempool does it for us.
	stateWithMetadata, err := l.memPool.StateMetadata(ledger.StoredStateReference(id))
	if err != nil {
		return l.ledgerState.ReadOutputByOutputID(id)
	}

	txWithMetadata, exists := l.memPool.TransactionMetadata(id.TransactionID())
	if !exists {
		return l.ledgerState.ReadOutputByOutputID(id)
	}

	earliestAttachment := txWithMetadata.EarliestIncludedAttachment()
	state, ok := stateWithMetadata.State().(*State)
	if !ok {
		return nil, ErrUnexpectedUnderlyingType
	}

	tx, ok := txWithMetadata.Transaction().(*Transaction)
	if !ok {
		return nil, ErrUnexpectedUnderlyingType
	}

	txCreationTime := tx.Transaction.Essence.CreationTime

	return ledgerstate.CreateOutput(l.ledgerState.API(), state.outputID, earliestAttachment, earliestAttachment.Index(), txCreationTime, state.output), nil
}

func (l *Ledger) CommitSlot(index iotago.SlotIndex) (stateRoot iotago.Identifier, mutationRoot iotago.Identifier, err error) {
	ledgerIndex, err := l.ledgerState.ReadLedgerIndex()
	if err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, err
	}

	if index != ledgerIndex+1 {
		panic(fmt.Errorf("there is a gap in the ledgerstate %d vs %d", ledgerIndex, index))
	}

	stateDiff := l.memPool.StateDiff(index)

	var innerErr error
	var outputs ledgerstate.Outputs
	var spents ledgerstate.Spents

	stateDiff.ExecutedTransactions().ForEach(func(txID iotago.TransactionID, txWithMeta mempool.TransactionMetadata) bool {
		tx, ok := txWithMeta.Transaction().(*Transaction)
		if !ok {
			innerErr = ErrUnexpectedUnderlyingType
			return false
		}
		txCreationTime := tx.Transaction.Essence.CreationTime

		inputs, errInput := tx.Inputs()
		if errInput != nil {
			innerErr = errInput
			return false
		}
		for _, input := range inputs {
			inputOutput, outputErr := l.Output(input.StateID())
			if outputErr != nil {
				innerErr = outputErr
				return false
			}

			spent := ledgerstate.NewSpent(inputOutput, txWithMeta.ID(), txCreationTime, index)
			spents = append(spents, spent)
		}

		if createOutputErr := txWithMeta.Outputs().ForEach(func(element mempool.StateMetadata) error {
			state, ok := element.State().(*State)
			if !ok {
				return ErrUnexpectedUnderlyingType
			}
			output := ledgerstate.CreateOutput(l.ledgerState.API(), state.outputID, txWithMeta.EarliestIncludedAttachment(), index, txCreationTime, state.output)
			outputs = append(outputs, output)

			return nil
		}); createOutputErr != nil {
			innerErr = createOutputErr
			return false
		}

		return true
	})

	if innerErr != nil {
		return iotago.Identifier{}, iotago.Identifier{}, innerErr
	}

	if err := l.ledgerState.ApplyDiff(index, outputs, spents); err != nil {
		return iotago.Identifier{}, iotago.Identifier{}, err
	}

	// Mark the transactions as committed so the mempool can evict it.
	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, tx mempool.TransactionMetadata) bool {
		tx.Commit()
		return true
	})

	return l.ledgerState.StateTreeRoot(), iotago.Identifier(stateDiff.Mutations().Root()), nil
}

func (l *Ledger) AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool) {
	switch payload := block.Block().Payload.(type) {
	case *iotago.Transaction:
		tx := &Transaction{payload}
		transactioMetadata, err := l.memPool.AttachTransaction(tx, block.ID())
		if err != nil {
			l.errorHandler(err)

			return nil, true
		}

		return transactioMetadata, true
	default:

		return nil, false
	}
}

func (l *Ledger) blockAccepted(block *blocks.Block) {
	switch block.Block().Payload.(type) {
	case *iotago.Transaction:
		l.memPool.MarkAttachmentIncluded(block.ID())

	default:
		return
	}
}
