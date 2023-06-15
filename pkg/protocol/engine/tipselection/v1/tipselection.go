package tipselectionv1

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipselection"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TipSelection struct {
	// rootBlocks is a function that returns the current root blocks.
	rootBlocks func() iotago.BlockIDs

	tipManager *TipManager

	// conflictDAG is the ConflictDAG that is used to track conflicts.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower]

	// memPool holds information about pending transactions.
	memPool mempool.MemPool[ledger.BlockVotePower]

	// events contains all the events that are triggered by the TipManager.
	events *tipselection.Events

	// optMaxStrongParents contains the maximum number of strong parents that are allowed.
	optMaxStrongParents int

	// optMaxLikedInsteadReferences contains the maximum number of liked instead references that are allowed.
	optMaxLikedInsteadReferences int

	// optMaxLikedInsteadReferencesPerParent contains the maximum number of liked instead references that are allowed
	// per parent.
	optMaxLikedInsteadReferencesPerParent int

	// optMaxWeakReferences contains the maximum number of weak references that are allowed.
	optMaxWeakReferences int

	module.Module
}

// NewProvider creates a new TipManager provider.
func NewProvider(opts ...options.Option[TipSelection]) module.Provider[*engine.Engine, tipselection.TipSelection] {
	return module.Provide(func(e *engine.Engine) tipselection.TipSelection {
		t := New(e.Ledger.ConflictDAG(), e.BlockCache.Block, e.EvictionState.LatestRootBlocks, opts...)

		e.Events.Booker.BlockBooked.Hook(lo.Void(t.AddBlock), event.WithWorkerPool(e.Workers.CreatePool("AddTip", 2)))
		e.BlockCache.Evict.Hook(t.tipManager.Evict)
		e.Events.TipManager.LinkTo(t.Events())

		t.TriggerInitialized()
		e.HookStopped(t.TriggerStopped)

		return t
	})
}

func New(conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower], blockRetriever func(blockID iotago.BlockID) (block *blocks.Block, exists bool), rootBlocksRetriever func() iotago.BlockIDs, opts ...options.Option[TipSelection]) *TipSelection {
	return options.Apply(&TipSelection{
		tipManager:                   NewTipManager(blockRetriever),
		events:                       tipselection.NewEvents(),
		conflictDAG:                  conflictDAG,
		rootBlocks:                   rootBlocksRetriever,
		optMaxStrongParents:          8,
		optMaxLikedInsteadReferences: 8,
		optMaxWeakReferences:         8,
	}, opts, func(t *TipSelection) {
		t.optMaxLikedInsteadReferencesPerParent = t.optMaxLikedInsteadReferences / 2
	}, (*TipSelection).TriggerConstructed)
}

func (t *TipSelection) AddBlock(block *blocks.Block) tipselection.TipMetadata {
	tipMetadata := t.tipManager.addBlock(block)
	if tipMetadata == nil {
		return nil
	}

	if t.isValidStrongTip(block) {
		tipMetadata.setTipPool(tipselection.StrongTipPool)
	} else if t.isValidWeakTip(block) {
		tipMetadata.setTipPool(tipselection.WeakTipPool)
	} else {
		tipMetadata.setTipPool(tipselection.DroppedTipPool)
	}

	t.events.BlockAdded.Trigger(tipMetadata)

	return tipMetadata
}

// SelectTips selects the references that should be used for block issuance.
func (t *TipSelection) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)
	_ = t.conflictDAG.ReadConsistent(func(_ conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVotePower]) error {
		t.collectStrongReferences(references, amount)
		t.collectWeakReferences(references)

		return nil
	})

	return references
}

// Events returns the events of the TipManager.
func (t *TipSelection) Events() *tipselection.Events {
	return t.events
}

func (t *TipSelection) TipManager() tipselection.TipManager {
	return t.tipManager
}

func (t *TipSelection) Shutdown() {
	// TODO: remove Shutdown from module.Interface
}

func (t *TipSelection) collectStrongReferences(references model.ParentReferences, amount int) {
	previousLikedInsteadConflicts := advancedset.New[iotago.TransactionID]()

	t.collectReferences(references, model.StrongParentType, t.tipManager.StrongTips, func(tip *TipMetadata) {
		addedLikedInsteadReferences, updatedLikedInsteadConflicts, err := t.likedInsteadReferences(previousLikedInsteadConflicts, tip)
		if err != nil {
			tip.setTipPool(tipselection.WeakTipPool)
		} else if len(addedLikedInsteadReferences) <= t.optMaxLikedInsteadReferences-len(references[model.ShallowLikeParentType]) {
			references[model.StrongParentType] = append(references[model.StrongParentType], tip.ID())
			references[model.ShallowLikeParentType] = append(references[model.ShallowLikeParentType], addedLikedInsteadReferences...)

			previousLikedInsteadConflicts = updatedLikedInsteadConflicts
		}
	}, amount)

	if len(references[model.StrongParentType]) == 0 {
		rootBlocks := t.rootBlocks()

		references[model.StrongParentType] = rootBlocks[:lo.Min(len(rootBlocks), t.optMaxStrongParents)]
	}
}

func (t *TipSelection) likedInsteadReferences(likedConflicts *advancedset.AdvancedSet[iotago.TransactionID], tipMetadata tipselection.TipMetadata) (references []iotago.BlockID, updatedLikedConflicts *advancedset.AdvancedSet[iotago.TransactionID], err error) {
	necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
	if err = t.conflictDAG.LikedInstead(tipMetadata.Block().ConflictIDs()).ForEach(func(likedConflictID iotago.TransactionID) error {
		transactionMetadata, exists := t.memPool.TransactionMetadata(likedConflictID)
		if !exists {
			return xerrors.Errorf("transaction required for liked instead reference (%s) not found in mem-pool", likedConflictID)
		}

		necessaryReferences[likedConflictID] = lo.First(transactionMetadata.Attachments())

		return nil
	}); err != nil {
		return nil, nil, err
	}

	references, updatedLikedConflicts = make([]iotago.BlockID, 0), likedConflicts.Clone()
	for conflictID, attachmentID := range necessaryReferences {
		if updatedLikedConflicts.Add(conflictID) {
			references = append(references, attachmentID)
		}
	}

	if len(references) > t.optMaxLikedInsteadReferencesPerParent {
		return nil, nil, xerrors.Errorf("too many liked instead references (%d) for block %s", len(references), tipMetadata.ID())
	}

	return references, updatedLikedConflicts, nil
}

func (t *TipSelection) collectWeakReferences(references model.ParentReferences) {
	t.collectReferences(references, model.WeakParentType, t.tipManager.WeakTips, func(tip *TipMetadata) {
		if !t.isValidWeakTip(tip.Block()) {
			tip.setTipPool(tipselection.DroppedTipPool)
		} else {
			references[model.WeakParentType] = append(references[model.WeakParentType], tip.ID())
		}
	}, t.optMaxWeakReferences)
}

func (t *TipSelection) collectReferences(references model.ParentReferences, parentsType model.ParentsType, tipSelector func(optAmount ...int) []tipselection.TipMetadata, callback func(*TipMetadata), amount int) {
	seenTips := advancedset.New[iotago.BlockID]()
	selectUniqueTips := func(amount int) (uniqueTips []tipselection.TipMetadata) {
		if amount <= 0 {
			return
		}

		for _, tip := range tipSelector(amount + seenTips.Size()) {
			if !seenTips.Add(tip.ID()) {
				continue
			}

			uniqueTips = append(uniqueTips, tip)
			if len(uniqueTips) == amount {
				break
			}
		}

		return uniqueTips
	}

	for tipCandidates := selectUniqueTips(amount); len(tipCandidates) != 0; tipCandidates = selectUniqueTips(amount - len(references[parentsType])) {
		for _, strongTip := range tipCandidates {
			if internalTip, ok := strongTip.(*TipMetadata); ok {
				callback(internalTip)
			}
		}
	}
}

func (t *TipSelection) isValidStrongTip(block *blocks.Block) bool {
	return !t.conflictDAG.AcceptanceState(block.ConflictIDs()).IsRejected()
}

func (t *TipSelection) isValidWeakTip(block *blocks.Block) bool {
	return t.conflictDAG.LikedInstead(block.PayloadConflictIDs()).Size() == 0
}

// WithMaxLikedInsteadReferences is an option for the TipSelection that allows to configure the maximum number of liked
// instead references.
func WithMaxLikedInsteadReferences(maxLikedInsteadReferences int) options.Option[TipSelection] {
	return func(tipManager *TipSelection) {
		tipManager.optMaxLikedInsteadReferences = maxLikedInsteadReferences
	}
}

// WithMaxWeakReferences is an option for the TipSelection that allows to configure the maximum number of weak references.
func WithMaxWeakReferences(maxWeakReferences int) options.Option[TipSelection] {
	return func(tipManager *TipSelection) {
		tipManager.optMaxWeakReferences = maxWeakReferences
	}
}
