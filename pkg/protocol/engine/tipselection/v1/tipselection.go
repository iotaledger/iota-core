package tipselectionv1

import (
	"time"

	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/core/agential"
	"github.com/iotaledger/iota-core/pkg/core/timed"
	"github.com/iotaledger/iota-core/pkg/model"
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

	// livenessThreshold holds the current liveness threshold.
	livenessThreshold *agential.ValueReceptor[time.Time]

	// optMaxStrongParents contains the maximum number of strong parents that are allowed.
	optMaxStrongParents int

	// optMaxLikedInsteadReferences contains the maximum number of liked instead references that are allowed.
	optMaxLikedInsteadReferences int

	// optMaxLikedInsteadReferencesPerParent contains the maximum number of liked instead references that are allowed
	// per parent.
	optMaxLikedInsteadReferencesPerParent int

	// optMaxWeakReferences contains the maximum number of weak references that are allowed.
	optMaxWeakReferences int

	// Module embeds the required methods of the module.Interface.
	module.Module
}

// New is the constructor for the TipSelection.
func New(tipManager tipmanager.TipManager, conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank], rootBlocksRetriever func() iotago.BlockIDs, opts ...options.Option[TipSelection]) *TipSelection {
	return options.Apply(&TipSelection{
		tipManager:                   tipManager,
		conflictDAG:                  conflictDAG,
		rootBlocks:                   rootBlocksRetriever,
		livenessThresholdQueue:       timed.NewPriorityQueue[tipmanager.TipMetadata](true),
		livenessThreshold:            agential.NewValueReceptor[time.Time](),
		optMaxStrongParents:          8,
		optMaxLikedInsteadReferences: 8,
		optMaxWeakReferences:         8,
	}, opts, func(t *TipSelection) {
		t.optMaxLikedInsteadReferencesPerParent = t.optMaxLikedInsteadReferences / 2

		t.livenessThreshold.OnUpdate(func(_, threshold time.Time) {
			for _, tip := range t.livenessThresholdQueue.PopUntil(threshold) {
				tip.IsLivenessThresholdReached().Set(true)
			}
		})

		t.TriggerConstructed()
	})
}

// SelectTips selects the tips that should be used as references for a new block.
func (t *TipSelection) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)
	_ = t.conflictDAG.ReadConsistent(func(_ conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, ledger.BlockVoteRank]) error {
		previousLikedInsteadConflicts := advancedset.New[iotago.TransactionID]()

		if t.collectReferences(references, model.StrongParentType, t.tipManager.StrongTips, func(tip tipmanager.TipMetadata) {
			addedLikedInsteadReferences, updatedLikedInsteadConflicts, err := t.likedInsteadReferences(previousLikedInsteadConflicts, tip)
			if err != nil {
				tip.TipPool().Set(tipmanager.WeakTipPool)
			} else if len(addedLikedInsteadReferences) <= t.optMaxLikedInsteadReferences-len(references[model.ShallowLikeParentType]) {
				references[model.StrongParentType] = append(references[model.StrongParentType], tip.ID())
				references[model.ShallowLikeParentType] = append(references[model.ShallowLikeParentType], addedLikedInsteadReferences...)

				previousLikedInsteadConflicts = updatedLikedInsteadConflicts
			}
		}, amount); len(references[model.StrongParentType]) == 0 {
			rootBlocks := t.rootBlocks()

			references[model.StrongParentType] = rootBlocks[:lo.Min(len(rootBlocks), t.optMaxStrongParents)]
		}

		t.collectReferences(references, model.WeakParentType, t.tipManager.WeakTips, func(tip tipmanager.TipMetadata) {
			if !t.isValidWeakTip(tip.Block()) {
				tip.TipPool().Set(tipmanager.DroppedTipPool)
			} else {
				references[model.WeakParentType] = append(references[model.WeakParentType], tip.ID())
			}
		}, t.optMaxWeakReferences)

		return nil
	})

	return references
}

// SetLivenessThreshold sets the liveness threshold used for tip selection (it can only increase monotonically).
func (t *TipSelection) SetLivenessThreshold(threshold time.Time) {
	t.livenessThreshold.Compute(func(currentThreshold time.Time) time.Time {
		return lo.Cond(threshold.Before(currentThreshold), currentThreshold, threshold)
	})
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

	t.livenessThresholdQueue.Push(tipMetadata, tipMetadata.Block().IssuingTime())
}

// likedInsteadReferences returns the liked instead references that are required to be able to reference the given tip.
func (t *TipSelection) likedInsteadReferences(likedConflicts *advancedset.AdvancedSet[iotago.TransactionID], tipMetadata tipmanager.TipMetadata) (references []iotago.BlockID, updatedLikedConflicts *advancedset.AdvancedSet[iotago.TransactionID], err error) {
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

// collectReferences collects tips from a tip selector (and calls the callback for each tip) until the amount of
// references of the given type is reached.
func (t *TipSelection) collectReferences(references model.ParentReferences, parentsType model.ParentsType, tipSelector func(optAmount ...int) []tipmanager.TipMetadata, callback func(tipmanager.TipMetadata), amount int) {
	seenTips := advancedset.New[iotago.BlockID]()
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
		for _, strongTip := range tipCandidates {
			callback(strongTip)
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
