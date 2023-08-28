package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	*model.Commitment

	chain        reactive.Variable[*Chain]
	parent       reactive.Variable[*Commitment]
	successor    reactive.Variable[*Commitment]
	spawnedChain reactive.Variable[*Chain]

	solid    reactive.Event
	attested reactive.Event
	verified reactive.Event
	evicted  reactive.Event

	parentVerified                        reactive.Event
	directlyAboveLatestVerifiedCommitment reactive.Variable[bool]
	parentAboveLatestVerifiedCommitment   reactive.Variable[bool]
	aboveLatestVerifiedCommitment         reactive.Variable[bool]

	belowSyncThreshold            reactive.Event
	belowWarpSyncThreshold        reactive.Event
	belowLatestVerifiedCommitment reactive.Event
	inSyncWindow                  reactive.Variable[bool]
	requiresWarpSync              reactive.Variable[bool]
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment: commitment,

		chain:        reactive.NewVariable[*Chain](),
		parent:       reactive.NewVariable[*Commitment](),
		successor:    reactive.NewVariable[*Commitment](),
		spawnedChain: reactive.NewVariable[*Chain](),

		solid:    reactive.NewEvent(),
		attested: reactive.NewEvent(),
		verified: reactive.NewEvent(),
		evicted:  reactive.NewEvent(),

		parentVerified:                      reactive.NewEvent(),
		parentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),

		belowSyncThreshold:            reactive.NewEvent(),
		belowWarpSyncThreshold:        reactive.NewEvent(),
		belowLatestVerifiedCommitment: reactive.NewEvent(),
	}

	c.chain.OnUpdate(func(_, chain *Chain) { chain.RegisterCommitment(c) })

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

func NewRootCommitment(commitment *model.Commitment) *Commitment {
	commitmentMetadata := NewCommitment(commitment)
	commitmentMetadata.SolidEvent().Trigger()
	commitmentMetadata.Verified().Trigger()
	commitmentMetadata.BelowSyncThreshold().Trigger()
	commitmentMetadata.BelowWarpSyncThreshold().Trigger()
	commitmentMetadata.BelowLatestVerifiedCommitment().Trigger()
	commitmentMetadata.Evicted().Trigger()

	return commitmentMetadata
}

func (c *Commitment) Chain() *Chain {
	return c.chain.Get()
}

func (c *Commitment) ChainVariable() reactive.Variable[*Chain] {
	return c.chain
}

func (c *Commitment) setChain(chain *Chain) {
	c.chain.Set(chain)
}

func (c *Commitment) Parent() *Commitment {
	return c.parent.Get()
}

func (c *Commitment) ParentVariable() reactive.Variable[*Commitment] {
	return c.parent
}

func (c *Commitment) setParent(parent *Commitment) {
	c.parent.Compute(func(currentParent *Commitment) *Commitment {
		if currentParent != nil {
			panic("parent may not be changed once it was set")
		}

		parent.registerChild(c, c.inheritChain(parent))

		c.registerParent(parent)

		return parent
	})
}

func (c *Commitment) IsSolid() bool {
	return c.solid.WasTriggered()
}

func (c *Commitment) SolidEvent() reactive.Event {
	return c.solid
}

func (c *Commitment) Attested() reactive.Event {
	return c.attested
}

func (c *Commitment) Verified() reactive.Event {
	return c.verified
}

func (c *Commitment) ParentVerified() reactive.Event {
	return c.parentVerified
}

func (c *Commitment) BelowSyncThreshold() reactive.Event {
	return c.belowSyncThreshold
}

func (c *Commitment) BelowWarpSyncThreshold() reactive.Event {
	return c.belowWarpSyncThreshold
}

func (c *Commitment) BelowLatestVerifiedCommitment() reactive.Event {
	return c.belowLatestVerifiedCommitment
}

func (c *Commitment) Evicted() reactive.Event {
	return c.evicted
}

func (c *Commitment) ParentAboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.parentAboveLatestVerifiedCommitment
}

func (c *Commitment) AboveLatestVerifiedCommitment() reactive.Variable[bool] {
	return c.aboveLatestVerifiedCommitment
}

func (c *Commitment) InSyncWindow() reactive.Variable[bool] {
	return c.inSyncWindow
}

func (c *Commitment) RequiresWarpSync() reactive.Variable[bool] {
	return c.requiresWarpSync
}

func (c *Commitment) ChainSuccessor() reactive.Variable[*Commitment] {
	return c.successor
}

// Max compares the given commitment with the current one and returns the one with the higher index.
func (c *Commitment) Max(latestCommitment *Commitment) *Commitment {
	if c == nil || latestCommitment != nil && latestCommitment.Index() >= c.Index() {
		return latestCommitment
	}

	return c
}

func (c *Commitment) registerParent(parent *Commitment) {
	c.solid.InheritFrom(parent.solid)
	c.parentVerified.InheritFrom(parent.verified)
	c.parentAboveLatestVerifiedCommitment.InheritFrom(parent.aboveLatestVerifiedCommitment)

	// triggerIfBelowThreshold triggers the given event if the commitment's index is below the given
	// threshold. We only monitor the threshold after the corresponding parent event was triggered (to minimize
	// the amount of elements that listen to updates of the same chain threshold - it spreads monotonically).
	triggerIfBelowThreshold := func(event func(*Commitment) reactive.Event, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
		event(parent).OnTrigger(func() {
			chainThreshold(c.Chain()).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
				event(c).Trigger()
			}, func(_ iotago.SlotIndex, slotIndex iotago.SlotIndex) bool {
				return c.Index() < slotIndex
			})
		})
	}

	triggerIfBelowThreshold((*Commitment).BelowLatestVerifiedCommitment, (*Chain).LatestVerifiedIndexVariable)
	triggerIfBelowThreshold((*Commitment).BelowSyncThreshold, (*Chain).SyncThresholdVariable)
	triggerIfBelowThreshold((*Commitment).BelowWarpSyncThreshold, (*Chain).WarpSyncThresholdVariable)
}

func (c *Commitment) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	c.successor.Compute(func(currentSuccessor *Commitment) *Commitment {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	// unsubscribe the handler on eviction to prevent memory leaks
	c.evicted.OnTrigger(c.successor.OnUpdate(onSuccessorUpdated))
}

func (c *Commitment) inheritChain(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, successor *Commitment) {
		c.spawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			switch successor {
			case nil:
				panic("successor may not be changed back to nil")
			case c:
				if spawnedChain != nil {
					spawnedChain.evicted.Trigger()
				}

				unsubscribeFromParent = parent.chain.OnUpdate(func(_, chain *Chain) { c.setChain(chain) })
			default:
				if spawnedChain != nil {
					return spawnedChain
				}

				if unsubscribeFromParent != nil {
					unsubscribeFromParent()
				}

				newSpawnedChain = NewChain(c)

				c.setChain(newSpawnedChain)
			}

			return newSpawnedChain
		})
	}
}
