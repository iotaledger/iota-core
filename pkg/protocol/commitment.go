package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Commitment struct {
	*model.Commitment

	Parent              reactive.Variable[*Commitment]
	Children            reactive.Set[*Commitment]
	MainChild           reactive.Variable[*Commitment]
	SpawnedChain        reactive.Variable[*Chain]
	Chain               reactive.Variable[*Chain]
	Engine              reactive.Variable[*engine.Engine]
	InSyncRange         *commitmentInSyncRange
	RequestBlocks       *commitmentRequestBlocks
	RequestAttestations reactive.Variable[bool]
	IsSolid             reactive.Event
	IsAttested          reactive.Event
	IsVerified          reactive.Event
	IsRoot              reactive.Event
	IsEvicted           reactive.Event
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment:          commitment,
		Parent:              reactive.NewVariable[*Commitment](),
		MainChild:           reactive.NewVariable[*Commitment](),
		Children:            reactive.NewSet[*Commitment](),
		SpawnedChain:        reactive.NewVariable[*Chain](),
		Chain:               reactive.NewVariable[*Chain](),
		Engine:              reactive.NewVariable[*engine.Engine](),
		RequestAttestations: reactive.NewVariable[bool](),
		IsSolid:             reactive.NewEvent(),
		IsAttested:          reactive.NewEvent(),
		IsVerified:          reactive.NewEvent(),
		IsRoot:              reactive.NewEvent(),
		IsEvicted:           reactive.NewEvent(),
	}

	c.deriveRequestAttestations()

	c.InSyncRange = newCommitmentInSyncRange(c)
	c.RequestBlocks = newCommitmentRequestBlocks(c)

	c.IsRoot.OnTrigger(func() {
		c.IsSolid.Set(true)
		c.IsAttested.Set(true)
		c.IsVerified.Set(true)
	})

	c.Parent.OnUpdate(func(_, parent *Commitment) {
		parent.registerChild(c, c.inheritChain(parent))

		c.IsSolid.InheritFrom(parent.IsSolid)
	})

	c.Chain.OnUpdate(func(_, chain *Chain) {
		chain.registerCommitment(c)

		// commitments inherit the engine from the chain
		chain.engine.OnUpdate(func(_, chainEngine *engine.Engine) {
			c.Engine.Set(chainEngine)
		})
	})

	return c
}

func (c *Commitment) deriveRequestAttestations() {
	isParentAttested := reactive.NewEvent()
	c.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		isParentAttested.InheritFrom(parent.IsAttested)
	})

	isDirectlyAboveLatestAttestedCommitment := reactive.NewDerivedVariable2(func(parentAttested, attested bool) bool {
		return parentAttested && !attested
	}, isParentAttested, c.IsAttested)

	c.Chain.OnUpdateWithContext(func(_, chain *Chain, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		withinContext(func() (unsubscribe func()) {
			attestationsRequested := reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
				return requestAttestations && isDirectlyAboveLatestAttestedCommitment
			}, chain.requestAttestations, isDirectlyAboveLatestAttestedCommitment)

			c.RequestAttestations.InheritFrom(attestationsRequested)

			return attestationsRequested.Unsubscribe
		})
	})
}

func (c *Commitment) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	if c.Children.Add(newChild) {
		c.MainChild.Compute(func(currentSuccessor *Commitment) *Commitment {
			return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
		})

		unsubscribe := c.MainChild.OnUpdate(onSuccessorUpdated)

		c.IsEvicted.OnTrigger(unsubscribe)
	}
}

func (c *Commitment) inheritChain(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, successor *Commitment) {
		c.SpawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			switch successor {
			case nil:
				panic("Successor may not be changed back to nil")

			case c:
				if spawnedChain != nil {
					spawnedChain.evicted.Trigger()
				}

				unsubscribeFromParent = parent.Chain.OnUpdate(func(_, chain *Chain) {
					c.Chain.Set(chain)
				})

				return nil

			default:
				if spawnedChain == nil {
					if unsubscribeFromParent != nil {
						unsubscribeFromParent()
					}

					spawnedChain = NewChain()
					spawnedChain.forkingPoint.Set(c)
				}

				return spawnedChain
			}
		})
	}
}

func (c *Commitment) promote(targetChain *Chain) {
	c.Chain.Compute(func(currentChain *Chain) *Chain {
		if currentChain != nil {
			fmt.Println("SHOULD PROMOTE")
			//currentChain.Promote(targetChain)
		}

		return targetChain
	})
}

type commitmentInSyncRange struct {
	reactive.Variable[bool]

	aboveLatestVerifiedCommitment reactive.Variable[bool]
	isBelowSyncThreshold          reactive.Event
}

func newCommitmentInSyncRange(commitment *Commitment) *commitmentInSyncRange {
	i := &commitmentInSyncRange{
		aboveLatestVerifiedCommitment: newCommitmentAboveLatestVerifiedCommitment(commitment),
		isBelowSyncThreshold:          reactive.NewEvent(),
	}

	commitment.Parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.InSyncRange.isBelowSyncThreshold
		}, commitment, (*Chain).SyncThreshold)
	})

	commitment.IsRoot.OnTrigger(func() {
		i.isBelowSyncThreshold.Set(true)
	})

	i.Variable = reactive.NewDerivedVariable2(func(belowSyncThreshold, aboveLatestVerifiedCommitment bool) bool {
		return belowSyncThreshold && aboveLatestVerifiedCommitment
	}, i.isBelowSyncThreshold, i.aboveLatestVerifiedCommitment)

	return i
}

func newCommitmentAboveLatestVerifiedCommitment(commitment *Commitment) reactive.Variable[bool] {
	var (
		parentVerified                        = reactive.NewEvent()
		parentAboveLatestVerifiedCommitment   = reactive.NewVariable[bool]()
		directlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(parentVerified, verified bool) bool {
			return parentVerified && !verified
		}, parentVerified, commitment.IsVerified)
	)

	commitment.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		parentVerified.InheritFrom(parent.IsVerified)
		parentAboveLatestVerifiedCommitment.InheritFrom(parent.InSyncRange.aboveLatestVerifiedCommitment)
	})

	return reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment)
}

type commitmentRequestBlocks struct {
	reactive.Variable[bool]

	isBelowWarpSyncThreshold reactive.Event
}

func newCommitmentRequestBlocks(commitment *Commitment) *commitmentRequestBlocks {
	r := &commitmentRequestBlocks{
		isBelowWarpSyncThreshold: reactive.NewEvent(),
	}

	commitment.IsRoot.OnTrigger(func() {
		r.isBelowWarpSyncThreshold.Set(true)
	})

	r.Variable = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, commitment.InSyncRange, r.isBelowWarpSyncThreshold)

	commitment.Parent.OnUpdate(func(_, parent *Commitment) {
		triggerEventIfCommitmentBelowThreshold(func(commitment *Commitment) reactive.Event {
			return commitment.RequestBlocks.isBelowWarpSyncThreshold
		}, commitment, (*Chain).WarpSyncThreshold)
	})

	return r
}
