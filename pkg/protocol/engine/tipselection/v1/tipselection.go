package tipselectionv1

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipSelection is a component that is used to abstract away the tip selection strategy, used to issue new blocks.
type TipSelection struct {
	// rootBlocks is a function that returns the current root blocks.
	rootBlocks func() iotago.BlockIDs

	// tipManager is the TipManager that is used to access the tip related metadata.
	tipManager tipmanager.TipManager

	// conflictDAG is the ConflictDAG that is used to track conflicts.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank]

	// memPool holds information about pending transactions.
	memPool mempool.MemPool[ledger.BlockVoteRank]

	// livenessThresholdQueue holds a queue of tips that are waiting to reach the liveness threshold.
	livenessThresholdQueue timed.PriorityQueue[tipmanager.TipMetadata]

	// acceptanceTime holds the current acceptance time.
	acceptanceTime reactive.Variable[time.Time]

	// optMaxStrongParents contains the maximum number of strong parents that are allowed.
	optMaxStrongParents int

	// optMaxLikedInsteadReferences contains the maximum number of liked instead references that are allowed.
	optMaxLikedInsteadReferences int

	// optMaxLikedInsteadReferencesPerParent contains the maximum number of liked instead references that are allowed
	// per parent.
	optMaxLikedInsteadReferencesPerParent int

	// optMaxWeakReferences contains the maximum number of weak references that are allowed.
	optMaxWeakReferences int

	// optDynamicLivenessThreshold is a function that is used to dynamically determine the liveness threshold for a tip.
	optDynamicLivenessThreshold func(tip tipmanager.TipMetadata) time.Duration

	// Module embeds the required methods of the module.Interface.
	module.Module
	engine *engine.Engine
}

// New is the constructor for the TipSelection.
func New(e *engine.Engine, tipManager tipmanager.TipManager, conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank], memPool mempool.MemPool[ledger.BlockVoteRank], rootBlocksRetriever func() iotago.BlockIDs, opts ...options.Option[TipSelection]) *TipSelection {
	return options.Apply(&TipSelection{
		tipManager:                   tipManager,
		engine:                       e,
		conflictDAG:                  conflictDAG,
		memPool:                      memPool,
		rootBlocks:                   rootBlocksRetriever,
		livenessThresholdQueue:       timed.NewPriorityQueue[tipmanager.TipMetadata](true),
		acceptanceTime:               reactive.NewVariable[time.Time](),
		optMaxStrongParents:          8,
		optMaxLikedInsteadReferences: 8,
		optMaxWeakReferences:         8,
		optDynamicLivenessThreshold: func(tip tipmanager.TipMetadata) time.Duration {
			// TODO: replace with correct version (LivenessThresholdLowerBound + approvalModifier * graceDuration)
			return e.CurrentAPI().LivenessThresholdDuration()
		},
	}, opts, func(t *TipSelection) {
		t.optMaxLikedInsteadReferencesPerParent = t.optMaxLikedInsteadReferences / 2

		t.acceptanceTime.OnUpdate(func(_, threshold time.Time) {
			for _, tip := range t.livenessThresholdQueue.PopUntil(threshold) {
				if dynamicLivenessThreshold := tip.Block().IssuingTime().Add(t.optDynamicLivenessThreshold(tip)); dynamicLivenessThreshold.After(threshold) {
					t.livenessThresholdQueue.Push(tip, dynamicLivenessThreshold)
				} else {
					tip.LivenessThresholdReached().Trigger()
				}
			}
		})

		t.TriggerConstructed()
	})
}

// SelectTips selects the tips that should be used as references for a new block.
func (t *TipSelection) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)
	strongParents := ds.NewSet[iotago.BlockID]()
	shallowLikesParents := ds.NewSet[iotago.BlockID]()
	_ = t.conflictDAG.ReadConsistent(func(_ conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank]) error {
		previousLikedInsteadConflicts := ds.NewSet[iotago.TransactionID]()

		if t.collectReferences(references, iotago.StrongParentType, t.tipManager.StrongTips, func(tip tipmanager.TipMetadata) {
			addedLikedInsteadReferences, updatedLikedInsteadConflicts, err := t.likedInsteadReferences(previousLikedInsteadConflicts, tip)
			if err != nil {
				tip.TipPool().Set(tipmanager.WeakTipPool)
			} else if len(addedLikedInsteadReferences) <= t.optMaxLikedInsteadReferences-len(references[iotago.ShallowLikeParentType]) {
				references[iotago.StrongParentType] = append(references[iotago.StrongParentType], tip.ID())
				references[iotago.ShallowLikeParentType] = append(references[iotago.ShallowLikeParentType], addedLikedInsteadReferences...)

				shallowLikesParents.AddAll(ds.NewSet(addedLikedInsteadReferences...))
				strongParents.Add(tip.ID())

				previousLikedInsteadConflicts = updatedLikedInsteadConflicts
			}
		}, amount); len(references[iotago.StrongParentType]) == 0 {
			rootBlocks := t.rootBlocks()

			references[iotago.StrongParentType] = rootBlocks[:lo.Min(len(rootBlocks), t.optMaxStrongParents)]
		}

		t.collectReferences(references, iotago.WeakParentType, t.tipManager.WeakTips, func(tip tipmanager.TipMetadata) {
			if !t.isValidWeakTip(tip.Block()) {
				tip.TipPool().Set(tipmanager.DroppedTipPool)
			} else if !shallowLikesParents.Has(tip.ID()) {
				references[iotago.WeakParentType] = append(references[iotago.WeakParentType], tip.ID())
			}
		}, t.optMaxWeakReferences)

		return nil
	})

	return references
}

// Shutdown does nothing but is required by the module.Interface.
func (t *TipSelection) Shutdown() {}

// classifyTip determines the initial tip pool of the given tip.
func (t *TipSelection) classifyTip(tipMetadata tipmanager.TipMetadata) {
	if t.isValidStrongTip(tipMetadata.Block()) {
		tipMetadata.TipPool().Set(tipmanager.StrongTipPool)
	} else if t.isValidWeakTip(tipMetadata.Block()) {
		tipMetadata.TipPool().Set(tipmanager.WeakTipPool)
	} else {
		tipMetadata.TipPool().Set(tipmanager.DroppedTipPool)
	}

	t.livenessThresholdQueue.Push(tipMetadata, tipMetadata.Block().IssuingTime().Add(t.optDynamicLivenessThreshold(tipMetadata)))
}

// likedInsteadReferences returns the liked instead references that are required to be able to reference the given tip.
func (t *TipSelection) likedInsteadReferences(likedConflicts ds.Set[iotago.TransactionID], tipMetadata tipmanager.TipMetadata) (references []iotago.BlockID, updatedLikedConflicts ds.Set[iotago.TransactionID], err error) {
	necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
	if err = t.conflictDAG.LikedInstead(tipMetadata.Block().ConflictIDs()).ForEach(func(likedConflictID iotago.TransactionID) error {
		transactionMetadata, exists := t.memPool.TransactionMetadata(likedConflictID)
		if !exists {
			return ierrors.Errorf("transaction required for liked instead reference (%s) not found in mem-pool", likedConflictID)
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
		return nil, nil, ierrors.Errorf("too many liked instead references (%d) for block %s", len(references), tipMetadata.ID())
	}

	return references, updatedLikedConflicts, nil
}

// collectReferences collects tips from a tip selector (and calls the callback for each tip) until the amount of
// references of the given type is reached.
func (t *TipSelection) collectReferences(references model.ParentReferences, parentsType iotago.ParentsType, tipSelector func(optAmount ...int) []tipmanager.TipMetadata, callback func(tipmanager.TipMetadata), amount int) {
	seenTips := ds.NewSet[iotago.BlockID]()
	selectUniqueTips := func(amount int) (uniqueTips []tipmanager.TipMetadata) {
		if amount > 0 {
			for _, tip := range tipSelector(amount + seenTips.Size()) {
				if seenTips.Add(tip.ID()) {
					uniqueTips = append(uniqueTips, tip)

					if len(uniqueTips) == amount {
						break
					}
				}
			}
		}

		return uniqueTips
	}

	for tipCandidates := selectUniqueTips(amount); len(tipCandidates) != 0; tipCandidates = selectUniqueTips(amount - len(references[parentsType])) {
		for _, tip := range tipCandidates {
			callback(tip)
		}
	}
}

// isValidStrongTip checks if the given block is a valid strong tip.
func (t *TipSelection) isValidStrongTip(block *blocks.Block) bool {
	return !t.conflictDAG.AcceptanceState(block.ConflictIDs()).IsRejected()
}

// isValidWeakTip checks if the given block is a valid weak tip.
func (t *TipSelection) isValidWeakTip(block *blocks.Block) bool {
	return t.conflictDAG.LikedInstead(block.PayloadConflictIDs()).Size() == 0
}

// WithMaxStrongParents is an option for the TipSelection that allows to configure the maximum number of strong parents.
func WithMaxStrongParents(maxStrongParents int) options.Option[TipSelection] {
	return func(tipManager *TipSelection) {
		tipManager.optMaxStrongParents = maxStrongParents
	}
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
