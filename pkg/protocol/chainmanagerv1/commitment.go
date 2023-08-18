package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	*model.Commitment

	chain                               reactive.Variable[*Chain]
	solid                               reactive.Event
	verified                            reactive.Event
	parentVerified                      reactive.Event
	belowSyncThreshold                  reactive.Event
	belowWarpSyncThreshold              reactive.Event
	belowLatestVerifiedCommitment       reactive.Event
	evicted                             reactive.Event
	parentAboveLatestVerifiedCommitment reactive.Variable[bool]
	aboveLatestVerifiedCommitment       reactive.Variable[bool]
	inSyncWindow                        reactive.Variable[bool]
	requiresWarpSync                    reactive.Variable[bool]
	chainSuccessor                      reactive.Variable[*Commitment]
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment: commitment,

		chain:                               reactive.NewVariable[*Chain](trapdoor),
		solid:                               reactive.NewEvent(),
		verified:                            reactive.NewEvent(),
		parentVerified:                      reactive.NewEvent(),
		belowSyncThreshold:                  reactive.NewEvent(),
		belowWarpSyncThreshold:              reactive.NewEvent(),
		belowLatestVerifiedCommitment:       reactive.NewEvent(),
		evicted:                             reactive.NewEvent(),
		parentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		chainSuccessor:                      reactive.NewVariable[*Commitment](),
	}

	c.aboveLatestVerifiedCommitment = reactive.NewDerivedVariable3(func(parentAboveLatestVerifiedCommitment, parentVerified, verified bool) bool {
		return parentAboveLatestVerifiedCommitment || (parentVerified && !verified)
	}, c.parentAboveLatestVerifiedCommitment, c.parentVerified, c.verified)

	c.inSyncWindow = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, c.belowSyncThreshold, c.aboveLatestVerifiedCommitment)

	c.requiresWarpSync = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.inSyncWindow, c.belowWarpSyncThreshold)

	return c
}

func (c *Commitment) RegisterParent(parent *Commitment) {
	parent.RegisterChild(c, c.inheritChain(parent))

	c.solid.InheritFrom(parent.solid)
	c.parentVerified.InheritFrom(parent.verified)
	c.parentAboveLatestVerifiedCommitment.InheritFrom(parent.aboveLatestVerifiedCommitment)

	// triggerEventIfIndexBelowThreshold triggers the given event if the commitment's index is below the given chain
	// threshold. It only monitors the chain threshold after the corresponding parent event was triggered (to avoid
	// unnecessary listeners  by the chain property to unaffected commitments).
	triggerEventIfIndexBelowThreshold := func(eventFunc func(*Commitment) reactive.Event, thresholdFunc func(*Chain) reactive.Variable[iotago.SlotIndex]) {
		eventFunc(parent).OnTrigger(func() {
			unsubscribe := thresholdFunc(c.chain.Get()).OnUpdate(func(_, latestVerifiedCommitmentIndex iotago.SlotIndex) {
				if c.Index() < latestVerifiedCommitmentIndex {
					eventFunc(c).Trigger()
				}
			})

			eventFunc(c).OnTrigger(unsubscribe)
		})
	}

	triggerEventIfIndexBelowThreshold((*Commitment).BelowLatestVerifiedCommitment, (*Chain).LatestVerifiedCommitmentIndex)
	triggerEventIfIndexBelowThreshold((*Commitment).BelowSyncThreshold, (*Chain).SyncThreshold)
	triggerEventIfIndexBelowThreshold((*Commitment).BelowWarpSyncThreshold, (*Chain).WarpSyncThreshold)
}

func (c *Commitment) RegisterChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	c.chainSuccessor.Compute(func(currentSuccessor *Commitment) *Commitment {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	unsubscribe := c.chainSuccessor.OnUpdate(onSuccessorUpdated)

	c.evicted.OnTrigger(unsubscribe)
}

// inheritChain returns a function that implements the chain inheritance rules.
//
// It must be called whenever the successor of the parent changes as we spawn a new chain for each child that is not the
// direct successor of a parent, and we inherit its chain otherwise.
func (c *Commitment) inheritChain(parent *Commitment) func(*Commitment, *Commitment) {
	var (
		spawnedChain *Chain
		unsubscribe  func()
	)

	return func(_, successor *Commitment) {
		switch successor {
		case nil:
			panic("successor must not be nil")
		case c:
			if spawnedChain != nil {
				// TODO: spawnedChain.Dispose()
				spawnedChain = nil
			}

			if unsubscribe == nil {
				unsubscribe = parent.chain.OnUpdate(func(_, chain *Chain) {
					c.chain.Set(chain)
				})
			}
		default:
			if unsubscribe != nil {
				unsubscribe()
				unsubscribe = nil
			}

			if spawnedChain == nil {
				spawnedChain = NewChain()

				c.chain.Set(spawnedChain)
			}
		}
	}
}
