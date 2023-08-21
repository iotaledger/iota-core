package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type CommitmentMetadata struct {
	*model.Commitment

	chain                                 reactive.Variable[*Chain]
	solid                                 reactive.Event
	verified                              reactive.Event
	parentVerified                        reactive.Event
	belowSyncThreshold                    reactive.Event
	belowWarpSyncThreshold                reactive.Event
	belowLatestVerifiedCommitment         reactive.Event
	evicted                               reactive.Event
	parentAboveLatestVerifiedCommitment   reactive.Variable[bool]
	directlyAboveLatestVerifiedCommitment reactive.Variable[bool]
	aboveLatestVerifiedCommitment         reactive.Variable[bool]
	inSyncWindow                          reactive.Variable[bool]
	requiresWarpSync                      reactive.Variable[bool]
	chainSuccessor                        reactive.Variable[*CommitmentMetadata]
}

func NewCommitmentMetadata(commitment *model.Commitment) *CommitmentMetadata {
	c := &CommitmentMetadata{
		Commitment: commitment,

		chain:                               reactive.NewVariable[*Chain](),
		solid:                               reactive.NewEvent(),
		verified:                            reactive.NewEvent(),
		parentVerified:                      reactive.NewEvent(),
		belowSyncThreshold:                  reactive.NewEvent(),
		belowWarpSyncThreshold:              reactive.NewEvent(),
		belowLatestVerifiedCommitment:       reactive.NewEvent(),
		evicted:                             reactive.NewEvent(),
		parentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		chainSuccessor:                      reactive.NewVariable[*CommitmentMetadata](),
	}

	c.chain.OnUpdate(func(_, chain *Chain) { chain.registerCommitment(c) })

	c.directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
		return parentVerified && !verified
	}, c.parentVerified, c.verified)

	c.aboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, c.directlyAboveLatestVerifiedCommitment, c.parentAboveLatestVerifiedCommitment)

	c.inSyncWindow = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, c.belowSyncThreshold, c.aboveLatestVerifiedCommitment)

	c.requiresWarpSync = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.inSyncWindow, c.belowWarpSyncThreshold)

	return c
}

func (c *CommitmentMetadata) Chain() reactive.Variable[*Chain] {
	return c.chain
}

func (c *CommitmentMetadata) Solid() reactive.Event {
	return c.solid
}

func (c *CommitmentMetadata) Verified() reactive.Event {
	return c.verified
}

func (c *CommitmentMetadata) ParentVerified() reactive.Event {
	return c.parentVerified
}

func (c *CommitmentMetadata) BelowSyncThreshold() reactive.Event {
	return c.belowSyncThreshold
}

func (c *CommitmentMetadata) BelowWarpSyncThreshold() reactive.Event {
	return c.belowWarpSyncThreshold
}

func (c *CommitmentMetadata) BelowLatestVerifiedCommitment() reactive.Event {
	return c.belowLatestVerifiedCommitment
}

func (c *CommitmentMetadata) Evicted() reactive.Event {
	return c.evicted
}

func (c *CommitmentMetadata) ParentAboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.parentAboveLatestVerifiedCommitment
}

func (c *CommitmentMetadata) AboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.aboveLatestVerifiedCommitment
}

func (c *CommitmentMetadata) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *CommitmentMetadata) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func (c *CommitmentMetadata) ChainSuccessor() reactive.Variable[*CommitmentMetadata] {
	return c.chainSuccessor
}

func (c *CommitmentMetadata) registerParent(parent *CommitmentMetadata) {
	parent.registerChild(c, c.inheritChain(parent))

	c.solid.InheritFrom(parent.solid)
	c.parentVerified.InheritFrom(parent.verified)
	c.parentAboveLatestVerifiedCommitment.InheritFrom(parent.aboveLatestVerifiedCommitment)

	// triggerEventIfIndexBelowThreshold triggers the given event if the commitment's index is below the given
	// threshold. We only monitor the threshold after the corresponding parent event was triggered (to minimize
	// the amount of elements that listen to updates of the same property).
	triggerEventIfIndexBelowThreshold := func(eventFunc func(*CommitmentMetadata) reactive.Event, thresholdFunc func(*Chain) reactive.Variable[iotago.SlotIndex]) {
		eventFunc(parent).OnTrigger(func() {
			unsubscribe := thresholdFunc(c.chain.Get()).OnUpdate(func(_, latestVerifiedCommitmentIndex iotago.SlotIndex) {
				if c.Index() < latestVerifiedCommitmentIndex {
					eventFunc(c).Trigger()
				}
			})

			eventFunc(c).OnTrigger(func() { go unsubscribe() })
		})
	}

	triggerEventIfIndexBelowThreshold((*CommitmentMetadata).BelowLatestVerifiedCommitment, (*Chain).LatestVerifiedCommitmentIndex)
	triggerEventIfIndexBelowThreshold((*CommitmentMetadata).BelowSyncThreshold, (*Chain).SyncThreshold)
	triggerEventIfIndexBelowThreshold((*CommitmentMetadata).BelowWarpSyncThreshold, (*Chain).WarpSyncThreshold)
}

func (c *CommitmentMetadata) registerChild(newChild *CommitmentMetadata, onSuccessorUpdated func(*CommitmentMetadata, *CommitmentMetadata)) {
	c.chainSuccessor.Compute(func(currentSuccessor *CommitmentMetadata) *CommitmentMetadata {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	// unsubscribe the handler on eviction to prevent memory leaks
	c.evicted.OnTrigger(c.chainSuccessor.OnUpdate(onSuccessorUpdated))
}

func (c *CommitmentMetadata) inheritChain(parent *CommitmentMetadata) func(*CommitmentMetadata, *CommitmentMetadata) {
	var spawnedChain *Chain
	spawnChain := func() {
		if spawnedChain == nil {
			spawnedChain = NewChain(c)
			c.chain.Set(spawnedChain)
		}
	}
	evictSpawnedChain := func() {
		if spawnedChain != nil {
			spawnedChain.evicted.Trigger()
			spawnedChain = nil
		}
	}

	var unsubscribe func()
	subscribeToParentChain := func() {
		if unsubscribe == nil {
			unsubscribe = parent.chain.OnUpdate(func(_, chain *Chain) { c.chain.Set(chain) })
		}
	}
	unsubscribeFromParentChain := func() {
		if unsubscribe != nil {
			unsubscribe()
			unsubscribe = nil
		}
	}

	return func(_, successor *CommitmentMetadata) {
		switch successor {
		case nil:
			panic("successor must never be changed back to nil")
		case c:
			evictSpawnedChain()
			subscribeToParentChain()
		default:
			unsubscribeFromParentChain()
			spawnChain()
		}
	}
}
