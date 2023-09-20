package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	*model.Commitment

	Parent                   reactive.Variable[*Commitment]
	Children                 reactive.Set[*Commitment]
	MainChild                reactive.Variable[*Commitment]
	SpawnedChain             reactive.Variable[*Chain]
	Chain                    reactive.Variable[*Chain]
	Engine                   reactive.Variable[*engine.Engine]
	InSyncRange              reactive.Variable[bool]
	WarpSyncBlocks           reactive.Variable[bool]
	RequestAttestations      reactive.Variable[bool]
	Weight                   reactive.Variable[uint64]
	AttestedWeight           reactive.Variable[uint64]
	CumulativeAttestedWeight reactive.Variable[uint64]
	IsSolid                  reactive.Event
	IsAttested               reactive.Event
	IsVerified               reactive.Event
	IsRoot                   reactive.Event
	IsEvicted                reactive.Event

	protocolLogger                          log.Logger
	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
	isAboveLatestVerifiedCommitment         reactive.Variable[bool]
	isBelowSyncThreshold                    reactive.Event
	isBelowWarpSyncThreshold                reactive.Event

	log.Logger
}

func NewCommitment(commitment *model.Commitment, logger log.Logger) *Commitment {
	c := &Commitment{
		Commitment: commitment,

		Parent:                   reactive.NewVariable[*Commitment](),
		MainChild:                reactive.NewVariable[*Commitment](),
		Children:                 reactive.NewSet[*Commitment](),
		SpawnedChain:             reactive.NewVariable[*Chain](),
		Chain:                    reactive.NewVariable[*Chain](),
		Engine:                   reactive.NewVariable[*engine.Engine](),
		RequestAttestations:      reactive.NewVariable[bool](),
		Weight:                   reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](func(current uint64, new uint64) uint64 { return max(current, new) }),
		CumulativeAttestedWeight: reactive.NewVariable[uint64](),
		IsSolid:                  reactive.NewEvent(),
		IsAttested:               reactive.NewEvent(),
		IsVerified:               reactive.NewEvent(),
		IsRoot:                   reactive.NewEvent(),
		IsEvicted:                reactive.NewEvent(),

		protocolLogger:                          logger,
		isDirectlyAboveLatestAttestedCommitment: reactive.NewVariable[bool](),
		isAboveLatestVerifiedCommitment:         reactive.NewVariable[bool](),
		isBelowSyncThreshold:                    reactive.NewEvent(),
		isBelowWarpSyncThreshold:                reactive.NewEvent(),
	}

	c.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		parent.registerChild(c)

		c.IsSolid.InheritFrom(parent.IsSolid)

		c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

		c.IsAttested.OnTrigger(func() {
			parent.IsAttested.OnTrigger(func() {
				c.CumulativeAttestedWeight.InheritFrom(reactive.NewDerivedVariable2(func(parentCumulativeAttestedWeight, attestedWeight uint64) uint64 {
					return parentCumulativeAttestedWeight + attestedWeight
				}, parent.CumulativeAttestedWeight, c.AttestedWeight))
			})
		})

		c.isDirectlyAboveLatestAttestedCommitment.InheritFrom(reactive.NewDerivedVariable2(func(parentIsAttested, isAttested bool) bool {
			return parentIsAttested && !isAttested
		}, parent.IsAttested, c.IsAttested))

		c.isAboveLatestVerifiedCommitment.InheritFrom(reactive.NewDerivedVariable3(func(parentIsAboveLatestVerifiedCommitment, parentIsVerified, isVerified bool) bool {
			return parentIsAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
		}, parent.isAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified))

		c.triggerEventIfBelowThreshold(
			func(c *Commitment) reactive.Event { return c.isBelowSyncThreshold },
			func(c *Chain) reactive.Variable[iotago.SlotIndex] { return c.SyncThreshold },
		)

		c.triggerEventIfBelowThreshold(
			func(c *Commitment) reactive.Event { return c.isBelowWarpSyncThreshold },
			func(c *Chain) reactive.Variable[iotago.SlotIndex] { return c.WarpSyncThreshold },
		)
	})

	c.Chain.OnUpdateWithContext(func(_, chain *Chain, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		withinContext(func() (unsubscribe func()) {
			requestAttestations := reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
				return requestAttestations && isDirectlyAboveLatestAttestedCommitment
			}, chain.RequestAttestations, c.isDirectlyAboveLatestAttestedCommitment)

			c.RequestAttestations.InheritFrom(requestAttestations)

			return lo.Batch(
				chain.registerCommitment(c),

				chain.Engine.OnUpdate(func(_, chainEngine *engine.Engine) {
					c.Engine.Set(chainEngine)
				}),

				requestAttestations.Unsubscribe,
			)
		})
	})

	c.InSyncRange = reactive.NewDerivedVariable2(func(aboveLatestVerifiedCommitment, belowSyncThreshold bool) bool {
		return aboveLatestVerifiedCommitment && belowSyncThreshold
	}, c.isAboveLatestVerifiedCommitment, c.isBelowSyncThreshold)

	c.WarpSyncBlocks = reactive.NewDerivedVariable2(func(inSyncWindow, belowWarpSyncThreshold bool) bool {
		return inSyncWindow && belowWarpSyncThreshold
	}, c.InSyncRange, c.isBelowWarpSyncThreshold)

	c.IsRoot.OnTrigger(func() {
		c.IsSolid.Set(true)
		c.IsAttested.Set(true)
		c.IsVerified.Set(true)
		c.isBelowWarpSyncThreshold.Set(true)
		c.isBelowSyncThreshold.Set(true)
	})

	c.Logger = logger.NewEntityLogger("Commitment", c.IsEvicted, func(entityLogger log.Logger) {
		c.Weight.LogUpdates(entityLogger, log.LevelTrace, "Weight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.CumulativeAttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "CumulativeAttestedWeight")
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
					spawnedChain.IsEvicted.Trigger()
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

					spawnedChain = NewChain(c.protocolLogger)
					spawnedChain.ForkingPoint.Set(c)

					c.Chain.Set(spawnedChain)
				}

				return spawnedChain
			}
		})
	}
}

func (c *Commitment) promote(targetChain *Chain) {
	c.Chain.Compute(func(currentChain *Chain) *Chain {
		if currentChain != nil && currentChain != targetChain {
			c.LogTrace("promoting commitment", "from", currentChain.LogName(), "to", targetChain.LogName())
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

func (c *Commitment) cumulativeWeight() uint64 {
	if c == nil {
		return 0
	}

	return c.CumulativeWeight()
}

func (c *Commitment) cumulativeAttestedWeight() uint64 {
	if c == nil {
		return 0
	}

	return c.CumulativeAttestedWeight.Get()
}
