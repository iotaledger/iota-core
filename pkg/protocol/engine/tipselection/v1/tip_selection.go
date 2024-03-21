package tipselectionv1

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/tipmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// TipSelection is a component that is used to abstract away the tip selection strategy, used to issue new blocks.
type TipSelection struct {
	// tipManager is the TipManager that is used to access the tip related metadata.
	tipManager tipmanager.TipManager

	// spendDAG is the SpendDAG that is used to track spenders.
	spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]

	// rootBlock is a function that returns the latest root block.
	rootBlock func() iotago.BlockID

	// livenessThreshold is a function that is used to determine the liveness threshold for a tip.
	livenessThreshold func(tip tipmanager.TipMetadata) time.Duration

	// transactionMetadata holds a function that is used to retrieve the metadata of a transaction.
	transactionMetadata func(iotago.TransactionID) (mempool.TransactionMetadata, bool)

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

	// livenessThresholdQueueMutex is used to synchronize access to the liveness threshold queue.
	livenessThresholdQueueMutex syncutils.RWMutex

	// acceptanceTimeMutex is used to synchronize access to the acceptance time.
	acceptanceTimeMutex syncutils.RWMutex

	// Module embeds the required methods of the modular framework.
	module.Module
}

// New is the constructor for the TipSelection.
func New(module module.Module, opts ...options.Option[TipSelection]) *TipSelection {
	return options.Apply(&TipSelection{
		Module:                                module,
		livenessThresholdQueue:                timed.NewPriorityQueue[tipmanager.TipMetadata](true),
		acceptanceTime:                        reactive.NewVariable[time.Time](monotonicallyIncreasing),
		optMaxStrongParents:                   8,
		optMaxLikedInsteadReferences:          8,
		optMaxLikedInsteadReferencesPerParent: 4,
		optMaxWeakReferences:                  8,
	}, opts)
}

// Construct fills in the dependencies of the TipSelection and triggers the constructed and initialized events of the
// module.
//
// This method is separated from the constructor so the TipSelection can be initialized lazily after all dependencies
// are available.
func (t *TipSelection) Construct(tipManager tipmanager.TipManager, spendDAG spenddag.SpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank], transactionMetadataRetriever func(iotago.TransactionID) (mempool.TransactionMetadata, bool), rootBlockRetriever func() iotago.BlockID, livenessThresholdFunc func(tipmanager.TipMetadata) time.Duration) *TipSelection {
	t.tipManager = tipManager
	t.spendDAG = spendDAG
	t.transactionMetadata = transactionMetadataRetriever
	t.rootBlock = rootBlockRetriever
	t.livenessThreshold = livenessThresholdFunc

	t.acceptanceTime.OnUpdate(func(_ time.Time, acceptanceTime time.Time) {
		t.triggerLivenessThreshold(acceptanceTime)
	})

	tipManager.OnBlockAdded(t.classifyTip)

	return module.InitSimpleLifecycle(t)
}

// SelectTips selects the tips that should be used as references for a new block.
func (t *TipSelection) SelectTips(amount int) (references model.ParentReferences) {
	references = make(model.ParentReferences)
	strongParents := ds.NewSet[iotago.BlockID]()
	shallowLikesParents := ds.NewSet[iotago.BlockID]()
	_ = t.spendDAG.ReadConsistent(func(_ spenddag.ReadLockedSpendDAG[iotago.TransactionID, mempool.StateID, ledger.BlockVoteRank]) error {
		previousLikedInsteadConflicts := ds.NewSet[iotago.TransactionID]()

		if t.collectReferences(func(tip tipmanager.TipMetadata) {
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
		}, func() int {
			return len(references[iotago.StrongParentType])
		},
			// We select one validation tip as a strong parent. This is a security step to ensure that the tangle maintains
			// acceptance by stitching together validation blocks.
			types.NewTuple[func(optAmount ...int) []tipmanager.TipMetadata, int](t.tipManager.ValidationTips, 2),
			types.NewTuple[func(optAmount ...int) []tipmanager.TipMetadata, int](t.tipManager.StrongTips, amount-2),
		); len(references[iotago.StrongParentType]) == 0 {
			references[iotago.StrongParentType] = iotago.BlockIDs{t.rootBlock()}
		}

		t.collectReferences(func(tip tipmanager.TipMetadata) {
			if !t.isValidWeakTip(tip.Block()) {
				tip.TipPool().Set(tipmanager.DroppedTipPool)
			} else if !shallowLikesParents.Has(tip.ID()) {
				references[iotago.WeakParentType] = append(references[iotago.WeakParentType], tip.ID())
			}
		}, func() int {
			return len(references[iotago.WeakParentType])
		}, types.NewTuple[func(optAmount ...int) []tipmanager.TipMetadata, int](t.tipManager.WeakTips, t.optMaxWeakReferences))

		return nil
	})

	return references
}

// SetAcceptanceTime updates the acceptance time of the TipSelection.
func (t *TipSelection) SetAcceptanceTime(acceptanceTime time.Time) (previousValue time.Time) {
	t.acceptanceTimeMutex.RLock()
	defer t.acceptanceTimeMutex.RUnlock()

	return t.acceptanceTime.Set(acceptanceTime)
}

// Reset resets the component to a clean state as if it was created at the last commitment.
func (t *TipSelection) Reset() {
	t.resetAcceptanceTime()
	t.resetLivenessThresholdQueue()
}

// classifyTip determines the initial tip pool of the given tip.
func (t *TipSelection) classifyTip(tipMetadata tipmanager.TipMetadata) {
	switch {
	case t.isValidStrongTip(tipMetadata.Block()):
		tipMetadata.TipPool().Set(tipmanager.StrongTipPool)

	case t.isValidWeakTip(tipMetadata.Block()):
		tipMetadata.TipPool().Set(tipmanager.WeakTipPool)

	default:
		tipMetadata.TipPool().Set(tipmanager.DroppedTipPool)
	}

	t.livenessThresholdQueueMutex.RLock()
	defer t.livenessThresholdQueueMutex.RUnlock()

	t.livenessThresholdQueue.Push(tipMetadata, tipMetadata.Block().IssuingTime().Add(t.livenessThreshold(tipMetadata)))
}

// likedInsteadReferences returns the liked instead references that are required to be able to reference the given tip.
func (t *TipSelection) likedInsteadReferences(likedConflicts ds.Set[iotago.TransactionID], tipMetadata tipmanager.TipMetadata) (references []iotago.BlockID, updatedLikedConflicts ds.Set[iotago.TransactionID], err error) {
	necessaryReferences := make(map[iotago.TransactionID]iotago.BlockID)
	if err = t.spendDAG.LikedInstead(tipMetadata.Block().SpenderIDs()).ForEach(func(likedSpenderID iotago.TransactionID) error {
		transactionMetadata, exists := t.transactionMetadata(likedSpenderID)
		if !exists {
			return ierrors.Errorf("transaction required for liked instead reference (%s) not found in mem-pool", likedSpenderID)
		}

		necessaryReferences[likedSpenderID] = lo.First(transactionMetadata.ValidAttachments())

		return nil
	}); err != nil {
		return nil, nil, err
	}

	references, updatedLikedConflicts = make([]iotago.BlockID, 0), likedConflicts.Clone()
	for spenderID, attachmentID := range necessaryReferences {
		if updatedLikedConflicts.Add(spenderID) {
			references = append(references, attachmentID)
		}
	}

	if len(references) > t.optMaxLikedInsteadReferencesPerParent {
		return nil, nil, ierrors.Errorf("too many liked instead references (%d) for block %s", len(references), tipMetadata.ID())
	}

	return references, updatedLikedConflicts, nil
}

// collectReferences collects tips from a tip selector (and calls the callback for each tip) until the number of
// references of the given type is reached.
func (t *TipSelection) collectReferences(callback func(tipmanager.TipMetadata), referencesCountCallback func() int, tipSelectorsAmount ...*types.Tuple[func(optAmount ...int) []tipmanager.TipMetadata, int]) {
	seenTips := ds.NewSet[iotago.BlockID]()

	// selectUniqueTips selects 'amount' unique tips from the given tip selector.
	selectUniqueTips := func(tipSelector func(optAmount ...int) []tipmanager.TipMetadata, currentReferencesCount int, targetAmount int) (uniqueTips []tipmanager.TipMetadata) {
		if targetAmount > 0 {
			for _, tip := range tipSelector(targetAmount + seenTips.Size()) {
				if seenTips.Add(tip.ID()) {
					uniqueTips = append(uniqueTips, tip)

					if currentReferencesCount+len(uniqueTips) == targetAmount {
						break
					}
				}
			}
		}

		return uniqueTips
	}

	accumulatedTipAmount := 0
	// We select the desired number of tips from all given tip selectors, respectively.
	for _, tipSelectorAmount := range tipSelectorsAmount {
		// Make sure we select the total number of unique tips and not just the number of tips from the given tip pool,
		// because of how selectUniqueTips works.
		accumulatedTipAmount += tipSelectorAmount.B

		tipCandidates := selectUniqueTips(tipSelectorAmount.A, referencesCountCallback(), accumulatedTipAmount)

		// We exit the loop in two cases:
		// 1. When we've seen all the tips and there are no more unique tips to process (len(tipCandidates) != 0).
		// 2. When we've successfully selected the desired number of tips and added them to the references (referencesCountCallback() >= accumulatedTipAmount).
		for len(tipCandidates) != 0 {
			for _, tip := range tipCandidates {
				callback(tip)
			}

			referencesCount := referencesCountCallback()
			if referencesCount >= accumulatedTipAmount {
				break
			}

			tipCandidates = selectUniqueTips(tipSelectorAmount.A, referencesCount, accumulatedTipAmount)
		}
	}
}

// isValidStrongTip checks if the given block is a valid strong tip.
func (t *TipSelection) isValidStrongTip(block *blocks.Block) bool {
	return !t.spendDAG.AcceptanceState(block.SpenderIDs()).IsRejected()
}

// isValidWeakTip checks if the given block is a valid weak tip.
func (t *TipSelection) isValidWeakTip(block *blocks.Block) bool {
	return t.spendDAG.LikedInstead(block.PayloadSpenderIDs()).Size() == 0
}

// triggerLivenessThreshold triggers the liveness threshold for all tips that have reached the given threshold.
func (t *TipSelection) triggerLivenessThreshold(threshold time.Time) {
	t.livenessThresholdQueueMutex.RLock()
	defer t.livenessThresholdQueueMutex.RUnlock()

	for _, tip := range t.livenessThresholdQueue.PopUntil(threshold) {
		if dynamicLivenessThreshold := tip.Block().IssuingTime().Add(t.livenessThreshold(tip)); dynamicLivenessThreshold.After(threshold) {
			t.livenessThresholdQueue.Push(tip, dynamicLivenessThreshold)
		} else {
			tip.LivenessThresholdReached().Trigger()
		}
	}
}

func (t *TipSelection) resetLivenessThresholdQueue() {
	t.livenessThresholdQueueMutex.Lock()
	defer t.livenessThresholdQueueMutex.Unlock()

	t.livenessThresholdQueue = timed.NewPriorityQueue[tipmanager.TipMetadata](true)
}

func (t *TipSelection) resetAcceptanceTime() {
	t.acceptanceTimeMutex.Lock()
	defer t.acceptanceTimeMutex.Unlock()

	t.acceptanceTime = reactive.NewVariable[time.Time](monotonicallyIncreasing)

	t.acceptanceTime.OnUpdate(func(_ time.Time, acceptanceTime time.Time) {
		t.triggerLivenessThreshold(acceptanceTime)
	})
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

// monotonicallyIncreasing returns the maximum of the two given times which is used as a transformation function to make
// the acceptance time of the TipSelection monotonically increasing.
func monotonicallyIncreasing(currentTime time.Time, newTime time.Time) time.Time {
	if currentTime.After(newTime) {
		return currentTime
	}

	return newTime
}
