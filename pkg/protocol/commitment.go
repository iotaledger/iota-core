package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	*model.Commitment

	Parent              reactive.Variable[*Commitment]
	Children            reactive.Set[*Commitment]
	MainChild           reactive.Variable[*Commitment]
	SpawnedChain        reactive.Variable[*Chain]
	Chain               reactive.Variable[*Chain]
	Engine              reactive.Variable[*engine.Engine]
	InSyncRange         reactive.Variable[bool]
	RequestBlocks       reactive.Variable[bool]
	RequestAttestations reactive.Variable[bool]
	IsSolid             reactive.Event
	IsAttested          reactive.Event
	IsVerified          reactive.Event
	IsRoot              reactive.Event
	IsEvicted           reactive.Event

	isParentAttested                        reactive.Event
	isParentVerified                        reactive.Event
	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
	isDirectlyAboveLatestVerifiedCommitment reactive.Variable[bool]
	isBelowSyncThreshold                    reactive.Event
	isBelowWarpSyncThreshold                reactive.Event
	isParentAboveLatestVerifiedCommitment   reactive.Variable[bool]
	isAboveLatestVerifiedCommitment         reactive.Variable[bool]
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment: commitment,

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

		isParentAttested:                      reactive.NewEvent(),
		isParentVerified:                      reactive.NewEvent(),
		isBelowSyncThreshold:                  reactive.NewEvent(),
		isBelowWarpSyncThreshold:              reactive.NewEvent(),
		isParentAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
	}

	c.isDirectlyAboveLatestAttestedCommitment = reactive.NewDerivedVariable2(func(isParentAttested, isAttested bool) bool {
		return isParentAttested && !isAttested
	}, c.isParentAttested, c.IsAttested)

	c.isDirectlyAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(isParentVerified, isVerified bool) bool {
		return isParentVerified && !isVerified
	}, c.isParentVerified, c.IsVerified)

	c.isAboveLatestVerifiedCommitment = reactive.NewDerivedVariable2(func(directlyAboveLatestVerifiedCommitment, parentAboveLatestVerifiedCommitment bool) bool {
		return directlyAboveLatestVerifiedCommitment || parentAboveLatestVerifiedCommitment
	}, c.isDirectlyAboveLatestVerifiedCommitment, c.isParentAboveLatestVerifiedCommitment)

	c.InSyncRange = reactive.NewDerivedVariable2(func(aboveLatestVerifiedCommitment, belowSyncThreshold bool) bool {
		return aboveLatestVerifiedCommitment && belowSyncThreshold
	}, c.isAboveLatestVerifiedCommitment, c.isBelowSyncThreshold)

	c.RequestBlocks = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.InSyncRange, c.isBelowWarpSyncThreshold)

	c.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		parent.registerChild(c)

		c.IsSolid.InheritFrom(parent.IsSolid)
		c.isParentAttested.InheritFrom(parent.IsAttested)
		c.isParentVerified.InheritFrom(parent.IsVerified)
		c.isParentAboveLatestVerifiedCommitment.InheritFrom(parent.isAboveLatestVerifiedCommitment)

		c.triggerEventIfBelowThreshold(func(c *Commitment) reactive.Event { return c.isBelowSyncThreshold }, (*Chain).SyncThreshold)
		c.triggerEventIfBelowThreshold(func(c *Commitment) reactive.Event { return c.isBelowWarpSyncThreshold }, (*Chain).WarpSyncThreshold)
	})

	c.Chain.OnUpdateWithContext(func(_, chain *Chain, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		chain.registerCommitment(c)

		withinContext(func() (unsubscribe func()) {
			return chain.engine.OnUpdate(func(_, chainEngine *engine.Engine) {
				c.Engine.Set(chainEngine)
			})
		})

		withinContext(func() (unsubscribe func()) {
			requestAttestations := reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
				return requestAttestations && isDirectlyAboveLatestAttestedCommitment
			}, chain.requestAttestations, c.isDirectlyAboveLatestAttestedCommitment)

			c.RequestAttestations.InheritFrom(requestAttestations)

			return requestAttestations.Unsubscribe
		})
	})

	c.IsRoot.OnTrigger(func() {
		c.IsSolid.Set(true)
		c.IsAttested.Set(true)
		c.IsVerified.Set(true)
		c.isBelowWarpSyncThreshold.Set(true)
		c.isBelowSyncThreshold.Set(true)
	})

	return c
}

func (c *Commitment) registerChild(child *Commitment) {
	if c.Children.Add(child) {
		c.MainChild.Compute(func(currentMainChild *Commitment) *Commitment {
			if currentMainChild != nil {
				return currentMainChild
			}

			return child
		})

		unsubscribeChild := c.MainChild.OnUpdate(child.inheritChain(c))

		c.IsEvicted.OnTrigger(unsubscribeChild)
	}
}

func (c *Commitment) inheritChain(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, mainChild *Commitment) {
		c.SpawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			switch mainChild {
			case nil:
				panic("main child may not be changed to nil")

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
					spawnedChain.ForkingPoint.Set(c)
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

func (c *Commitment) triggerEventIfBelowThreshold(event func(*Commitment) reactive.Event, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
	c.Chain.OnUpdateWithContext(func(_, chain *Chain, withinContext func(subscriptionFactory func() (unsubscribe func()))) {

		// only monitor the threshold after the parent event was triggered (minimize listeners to same threshold)
		withinContext(func() (unsubscribe func()) {
			return event(c.Parent.Get()).OnTrigger(func() {
				// since events only trigger once, we unsubscribe from the threshold after the trigger condition is met
				chainThreshold(chain).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
					event(c).Trigger()
				}, func(_, slotIndex iotago.SlotIndex) bool {
					return c.Index() < slotIndex
				})
			})
		})
	})
}
