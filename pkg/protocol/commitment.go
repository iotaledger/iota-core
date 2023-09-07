package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Commitment struct {
	*model.Commitment

	parent              reactive.Variable[*Commitment]
	successor           reactive.Variable[*Commitment]
	children            reactive.Set[*Commitment]
	spawnedChain        reactive.Variable[*Chain]
	chain               reactive.Variable[*Chain]
	solid               reactive.Event
	attested            reactive.Event
	verified            reactive.Event
	inSyncRange         *commitmentInSyncRange
	requestBlocks       *commitmentRequestBlocks
	requestAttestations *commitmentRequestAttestations
	evicted             reactive.Event
}

func NewCommitment(commitment *model.Commitment, optIsRoot ...bool) *Commitment {
	c := &Commitment{
		Commitment:   commitment,
		parent:       reactive.NewVariable[*Commitment](),
		successor:    reactive.NewVariable[*Commitment](),
		children:     reactive.NewSet[*Commitment](),
		spawnedChain: reactive.NewVariable[*Chain](),
		chain:        reactive.NewVariable[*Chain](),
		solid:        reactive.NewEvent(),
		attested:     reactive.NewEvent(),
		verified:     reactive.NewEvent(),
		evicted:      reactive.NewEvent(),
	}

	c.inSyncRange = newCommitmentInSyncRange(c, lo.First(optIsRoot))
	c.requestBlocks = newCommitmentRequestBlocks(c, lo.First(optIsRoot))
	c.requestAttestations = newCommitmentRequestAttestations(c)

	c.parent.OnUpdate(func(_, parent *Commitment) { c.solid.InheritFrom(parent.solid) })

	c.chain.OnUpdate(func(_, chain *Chain) { chain.registerCommitment(c) })

	if lo.First(optIsRoot) {
		c.solid.Set(true)
		c.attested.Set(true)
		c.verified.Set(true)
	}

	return c
}

func (c *Commitment) CommitmentModel() *model.Commitment {
	return c.Commitment
}

func (c *Commitment) Parent() reactive.Variable[*Commitment] {
	return c.parent
}

func (c *Commitment) Successor() reactive.Variable[*Commitment] {
	return c.successor
}

func (c *Commitment) Children() reactive.Set[*Commitment] {
	return c.children
}

func (c *Commitment) SpawnedChain() reactive.Variable[*Chain] {
	return c.spawnedChain
}

func (c *Commitment) Chain() *Chain {
	return c.chain.Get()
}

func (c *Commitment) ChainR() reactive.Variable[*Chain] {
	return c.chain
}

func (c *Commitment) Solid() reactive.Event {
	return c.solid
}

func (c *Commitment) Attested() reactive.Event {
	return c.attested
}

func (c *Commitment) Verified() reactive.Event {
	return c.verified
}

func (c *Commitment) InSyncRange() reactive.Variable[bool] {
	return c.inSyncRange
}

func (c *Commitment) RequestBlocks() reactive.Variable[bool] {
	return c.requestBlocks
}

func (c *Commitment) RequestAttestations() reactive.Variable[bool] {
	return c.requestAttestations
}

func (c *Commitment) Engine() reactive.Variable[*engine.Engine] {
	return c.chain.Get().EngineR()
}

func (c *Commitment) Evicted() reactive.Event {
	return c.evicted
}

func (c *Commitment) setParent(parent *Commitment) {
	parent.registerChild(c, c.chainUpdater(parent))

	if c.parent.Set(parent) != nil {
		panic("parent may not be changed once it was set")
	}
}

func (c *Commitment) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	if c.children.Add(newChild) {
		c.successor.Compute(func(currentSuccessor *Commitment) *Commitment {
			return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
		})

		unsubscribe := c.successor.OnUpdate(onSuccessorUpdated)

		c.evicted.OnTrigger(unsubscribe)
	}
}

func (c *Commitment) chainUpdater(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, successor *Commitment) {
		c.spawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			if successor == nil {
				panic("successor may not be changed back to nil")
			}

			if successor == c {
				if spawnedChain != nil {
					spawnedChain.evicted.Trigger()
				}

				unsubscribeFromParent = parent.chain.OnUpdate(func(_, chain *Chain) { c.chain.Set(chain) })

				return nil
			}

			if spawnedChain != nil {
				return spawnedChain
			}

			if unsubscribeFromParent != nil {
				unsubscribeFromParent()
			}

			return NewChain(c)
		})
	}
}

// max compares the commitment with the given other commitment and returns the one with the higher index.
func (c *Commitment) max(other *Commitment) *Commitment {
	if c == nil || other != nil && other.Index() >= c.Index() {
		return other
	}

	return c
}

func (c *Commitment) cumulativeWeight() uint64 {
	if c == nil {
		return 0
	}

	return c.CumulativeWeight()
}

type commitmentInSyncRange struct {
	reactive.Variable[bool]

	aboveLatestVerifiedCommitment *commitmentAboveLatestVerifiedCommitment
	isBelowSyncThreshold          reactive.Event
}

func newCommitmentInSyncRange(commitment *Commitment, isRoot bool) *commitmentInSyncRange {
	i := &commitmentInSyncRange{
		aboveLatestVerifiedCommitment: newCommitmentAboveLatestVerifiedCommitment(commitment),
		isBelowSyncThreshold:          reactive.NewEvent(),
	}

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.inSyncRange.isBelowSyncThreshold
		}, commitment, (*Chain).SyncThreshold)
	})

	if isRoot {
		i.isBelowSyncThreshold.Set(true)
	}

	i.Variable = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, i.isBelowSyncThreshold, newCommitmentAboveLatestVerifiedCommitment(commitment))

	return i
}

type commitmentAboveLatestVerifiedCommitment struct {
	reactive.Variable[bool]

	parentVerified reactive.Event

	parentAboveLatestVerifiedCommitment reactive.Variable[bool]

	directlyAboveLatestVerifiedCommitment reactive.Variable[bool]
}

func newCommitmentAboveLatestVerifiedCommitment(commitment *Commitment) *commitmentAboveLatestVerifiedCommitment {
	a := &commitmentAboveLatestVerifiedCommitment{
		parentVerified:                      reactive.NewEvent(),
		parentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
	}

	a.directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
		return parentVerified && !verified
	}, a.parentVerified, commitment.verified)

	a.Variable = reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, a.directlyAboveLatestVerifiedCommitment, a.parentAboveLatestVerifiedCommitment)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		a.parentVerified.InheritFrom(parent.verified)
		a.parentAboveLatestVerifiedCommitment.InheritFrom(parent.inSyncRange.aboveLatestVerifiedCommitment)
	})

	return a
}

type commitmentRequestAttestations struct {
	reactive.Variable[bool]

	parentAttested reactive.Event

	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
}

func newCommitmentRequestAttestations(commitment *Commitment) *commitmentRequestAttestations {
	c := &commitmentRequestAttestations{
		Variable:       reactive.NewVariable[bool](),
		parentAttested: reactive.NewEvent(),
	}

	c.isDirectlyAboveLatestAttestedCommitment = reactive.NewDerivedVariable2(func(parentAttested, attested bool) bool {
		return parentAttested && !attested
	}, c.parentAttested, commitment.attested)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) { c.parentAttested.InheritFrom(parent.attested) })

	var attestationRequestedByChain reactive.DerivedVariable[bool]

	commitment.chain.OnUpdate(func(_, newChain *Chain) {
		// cleanup the old chain specific derived variable if it exists
		if attestationRequestedByChain != nil {
			attestationRequestedByChain.Unsubscribe()
		}

		// create a chain specific derived variable
		attestationRequestedByChain = reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
			return requestAttestations && isDirectlyAboveLatestAttestedCommitment
		}, newChain.requestAttestations, c.isDirectlyAboveLatestAttestedCommitment)

		// expose the chain specific derived variable to the commitment property
		c.InheritFrom(attestationRequestedByChain)
	})

	return c
}

type commitmentRequestBlocks struct {
	reactive.Variable[bool]

	isBelowWarpSyncThreshold reactive.Event
}

func newCommitmentRequestBlocks(commitment *Commitment, isRoot bool) *commitmentRequestBlocks {
	r := &commitmentRequestBlocks{
		isBelowWarpSyncThreshold: reactive.NewEvent(),
	}

	if isRoot {
		r.isBelowWarpSyncThreshold.Set(true)
	}

	r.Variable = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, commitment.inSyncRange, r.isBelowWarpSyncThreshold)

	commitment.parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.requestBlocks.isBelowWarpSyncThreshold
		}, commitment, (*Chain).WarpSyncThreshold)
	})

	return r
}
