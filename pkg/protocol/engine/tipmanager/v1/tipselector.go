package tipmanagerv1

import (
	"golang.org/x/xerrors"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

type TipSelector struct {
	// retrieveRootBlocks is a function that returns the current root blocks.
	retrieveRootBlocks func() iotago.BlockIDs

	tipManager tipmanager.TipManager

	// conflictDAG is the ConflictDAG that is used to track conflicts.
	conflictDAG conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]

	// memPool holds information about pending transactions.
	memPool mempool.MemPool[booker.BlockVotePower]

	// optMaxStrongParents contains the maximum number of strong parents that are allowed.
	optMaxStrongParents int

	// optMaxLikedInsteadReferences contains the maximum number of liked instead references that are allowed.
	optMaxLikedInsteadReferences int

	// optMaxLikedInsteadReferencesPerParent contains the maximum number of liked instead references that are allowed
	// per parent.
	optMaxLikedInsteadReferencesPerParent int

	// optMaxWeakReferences contains the maximum number of weak references that are allowed.
	optMaxWeakReferences int
}

func NewTipSelector(rootBlocksRetriever func() iotago.BlockIDs, opts ...options.Option[TipSelector]) *TipSelector {
	return options.Apply(&TipSelector{
		retrieveRootBlocks:           rootBlocksRetriever,
		optMaxStrongParents:          8,
		optMaxLikedInsteadReferences: 8,
		optMaxWeakReferences:         8,
	}, opts, func(t *TipSelector) {
		t.optMaxLikedInsteadReferencesPerParent = t.optMaxLikedInsteadReferences / 2
	})
}

// SelectTips selects the references that should be used for block issuance.
func (t *TipSelector) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)
	_ = t.conflictDAG.ReadConsistent(func(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]) error {
		t.collectStrongTips(conflictDAG, references, amount)
		t.collectWeakTips(references, amount)

		return nil
	})

	return references
}

func (t *TipSelector) collectStrongTips(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower], references model.ParentReferences, amount int) {
	likedInsteadConflicts := advancedset.New[iotago.TransactionID]()
	t.collectReferences(references, model.StrongParentType, t.tipManager.StrongTips, func(tip tipmanager.TipMetadata) {
		addedLikedInsteadReferences, updatedLikedInsteadConflicts, err := t.likedInsteadReferences(conflictDAG, likedInsteadConflicts, tip)
		if err != nil {
			tip.SetTipPool(tipmanager.WeakTipPool)

			return
		} else if len(addedLikedInsteadReferences) > t.optMaxLikedInsteadReferences-len(references[model.ShallowLikeParentType]) {
			return
		}

		references[model.StrongParentType] = append(references[model.StrongParentType], tip.ID())
		references[model.ShallowLikeParentType] = append(references[model.ShallowLikeParentType], addedLikedInsteadReferences...)

		likedInsteadConflicts = updatedLikedInsteadConflicts
	}, amount)

	if len(references[model.StrongParentType]) == 0 {
		rootBlocks := t.retrieveRootBlocks()

		references[model.StrongParentType] = rootBlocks[:lo.Max(len(rootBlocks), t.optMaxStrongParents)]
	}
}

func (t *TipSelector) likedInsteadReferences(conflictDAG conflictdag.ReadLockedConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower], likedConflicts *advancedset.AdvancedSet[iotago.TransactionID], tipMetadata tipmanager.TipMetadata) (references []iotago.BlockID, updatedLikedConflicts *advancedset.AdvancedSet[iotago.TransactionID], err error) {
	necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
	if err = conflictDAG.LikedInstead(tipMetadata.Block().ConflictIDs()).ForEach(func(likedConflictID iotago.TransactionID) error {
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

func (t *TipSelector) collectWeakTips(references model.ParentReferences, amount int) {
	t.collectReferences(references, model.WeakParentType, t.tipManager.WeakTips, func(tip tipmanager.TipMetadata) {
		if !t.isValidWeakTip(tip.Block()) {
			tip.SetTipPool(tipmanager.DroppedTipPool)
		} else {
			references[model.WeakParentType] = append(references[model.WeakParentType], tip.ID())
		}
	}, t.optMaxWeakReferences)
}

func (t *TipSelector) collectReferences(references model.ParentReferences, parentsType model.ParentsType, tipSelector func(optAmount ...int) []tipmanager.TipMetadata, callback func(tipmanager.TipMetadata), amount int) {
	selectTips := t.uniqueTipSelector(tipSelector)
	for tipCandidates := selectTips(amount); len(tipCandidates) != 0; tipCandidates = selectTips(amount - len(references[parentsType])) {
		for _, strongTip := range tipCandidates {
			callback(strongTip)
		}
	}
}

func (t *TipSelector) uniqueTipSelector(tipSelector func(optAmount ...int) []tipmanager.TipMetadata) func(amount int) []tipmanager.TipMetadata {
	seenTips := advancedset.New[iotago.BlockID]()

	return func(amount int) (uniqueTips []tipmanager.TipMetadata) {
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
}

func (t *TipSelector) isValidWeakTip(block *blocks.Block) bool {
	return t.conflictDAG.LikedInstead(block.PayloadConflictIDs()).Size() == 0
}

// WithMaxLikedInsteadReferences is an option for the TipSelector that allows to configure the maximum number of liked
// instead references.
func WithMaxLikedInsteadReferences(maxLikedInsteadReferences int) options.Option[TipSelector] {
	return func(tipManager *TipSelector) {
		tipManager.optMaxLikedInsteadReferences = maxLikedInsteadReferences
	}
}

// WithMaxWeakReferences is an option for the TipSelector that allows to configure the maximum number of weak references.
func WithMaxWeakReferences(maxWeakReferences int) options.Option[TipSelector] {
	return func(tipManager *TipSelector) {
		tipManager.optMaxWeakReferences = maxWeakReferences
	}
}
