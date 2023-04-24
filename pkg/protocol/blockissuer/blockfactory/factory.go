package blockfactory

import (
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

// region Factory ///////////////////////////////////////////////////////////////////////////////////////////////

// Factory acts as a factory to create new blocks.
type Factory struct {
	Events *Events

	// referenceProvider *ReferenceProvider
	localPrivateKey ed25519.PrivateKey
	api             iotago.API
	blockRetriever  func(blockID iotago.BlockID) (block *blocks.Block, exists bool)
	tipManager      tipmanager.TipManager
	referencesFunc  ReferencesFunc
	commitmentFunc  CommitmentFunc

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
}

// NewBlockFactory creates a new block factory.
func NewBlockFactory(localPrivateKey ed25519.PrivateKey, tipManager tipmanager.TipManager, referencesFunc ReferencesFunc, commitmentFunc CommitmentFunc, opts ...options.Option[Factory]) *Factory {
	return options.Apply(&Factory{
		Events:          newEvents(),
		localPrivateKey: localPrivateKey,
		tipManager:      tipManager,
		referencesFunc:  referencesFunc,
		commitmentFunc:  commitmentFunc,

		optsTipSelectionTimeout:       10 * time.Second,
		optsTipSelectionRetryInterval: 200 * time.Millisecond,
	}, opts)
}

// CreateBlock creates a new block including sequence number and tip selection and returns it.
func (f *Factory) CreateBlock(p iotago.Payload, parentsCount ...int) (*model.Block, error) {
	return f.CreateBlockWithReferences(p, nil, parentsCount...)
}

// CreateBlockWithReferences creates a new block with the references submit.
func (f *Factory) CreateBlockWithReferences(p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (*model.Block, error) {
	strongParentsCount := iotago.BlockMinStrongParents
	if len(strongParentsCountOpt) > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	var err error
	if references == nil {
		references, err = f.getReferencesWithRetry(p, strongParentsCount)
		if err != nil {
			return nil, errors.Wrap(err, "error while trying to get references")
		}
	}

	slotCommitment, lastFinalizedSlot := f.commitmentFunc()
	pubKey := f.localPrivateKey.Public()
	addr := iotago.Ed25519AddressFromPubKey(pubKey[:])

	// TODO: do we have to ensure that the issuing time is at least the Max (latest) of the parents' issuing times?
	block, err := builder.NewBlockBuilder().
		StrongParents(references[model.StrongParentType]).
		WeakParents(references[model.WeakParentType]).
		ShallowLikeParents(references[model.ShallowLikeParentType]).
		SlotCommitment(slotCommitment).
		LatestFinalizedSlot(lastFinalizedSlot).
		Payload(p).
		Sign(&addr, f.localPrivateKey[:]).
		Build()
	if err != nil {
		return nil, errors.Wrap(err, "error building block")
	}

	modelBlock, err := model.BlockFromBlock(block, f.api)
	if err != nil {
		return nil, errors.Wrap(err, "error serializing block to model block")
	}

	f.Events.BlockConstructed.Trigger(modelBlock)
	return modelBlock, nil
}

// getReferencesWithRetry tries to get references for the given payload. If it fails, it will retry at regular intervals until
// the timeout is reached.
func (f *Factory) getReferencesWithRetry(p iotago.Payload, parentsCount int) (references model.ParentReferences, err error) {
	references, err = f.getReferences(p, parentsCount)
	if err == nil {
		return references, nil
	}
	f.Events.Error.Trigger(errors.Wrap(err, "could not get references"))

	timeout := time.NewTimer(f.optsTipSelectionTimeout)
	interval := time.NewTicker(f.optsTipSelectionRetryInterval)
	for {
		select {
		case <-interval.C:
			references, err = f.getReferences(p, parentsCount)
			if err != nil {
				f.Events.Error.Trigger(errors.Wrap(err, "could not get references"))
				continue
			}

			return references, nil
		case <-timeout.C:
			return nil, errors.Errorf("timeout while trying to select tips and determine references")
		}
	}
}

func (f *Factory) getReferences(p iotago.Payload, parentsCount int) (references model.ParentReferences, err error) {
	strongParents := f.tips(p, parentsCount)
	if len(strongParents) == 0 {
		return nil, errors.Errorf("no strong parents were selected in tip selection")
	}

	references, err = f.referencesFunc(p, strongParents)
	// If none of the strong parents are possible references, we have to try again.
	if err != nil {
		return nil, errors.Wrap(err, "references could not be created")
	}

	// TODO: should we remove duplicates in the references? It shouldn't be necessary with the new ConflictDAG.
	return references, nil
}

// TODO: when Ledger is refactored, we need to rework the stuff below
func (f *Factory) tips(p iotago.Payload, parentsCount int) (parents iotago.BlockIDs) {
	parents = f.tipManager.Tips(parentsCount)

	// tx, ok := p.(utxo.Transaction)
	// if !ok {
	// 	return parents
	// }

	// If the block is issuing a transaction and is a double spend, we add it in parallel to the earliest attachment
	// to prevent a double spend from being issued in its past cone.
	// if conflictingTransactions := f.tangle.Ledger.Utils.ConflictingTransactions(tx.ID()); !conflictingTransactions.IsEmpty() {
	//	if earliestAttachment := f.EarliestAttachment(conflictingTransactions); earliestAttachment != nil {
	//		return earliestAttachment.ParentsByType(tangle.StrongParentType)
	//	}
	// }

	return parents
}

// ReferencesFunc is a function type that returns like references a given set of parents of a Block.
type ReferencesFunc func(payload iotago.Payload, strongParents iotago.BlockIDs) (references model.ParentReferences, err error)

// CommitmentFunc is a function type that returns the commitment of the latest committable slot.
type CommitmentFunc func() (commitment *iotago.Commitment, lastFinalizedSlot iotago.SlotIndex)

func WithTipSelectionTimeout(timeout time.Duration) options.Option[Factory] {
	return func(factory *Factory) {
		factory.optsTipSelectionTimeout = timeout
	}
}

func WithTipSelectionRetryInterval(interval time.Duration) options.Option[Factory] {
	return func(factory *Factory) {
		factory.optsTipSelectionRetryInterval = interval
	}
}
