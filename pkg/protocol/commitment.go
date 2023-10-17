package protocol

import (
	"fmt"

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
	RequestAttestations      reactive.Variable[bool]
	RequestBlocks            reactive.Variable[bool]
	RequestedBlocksReceived  reactive.Variable[bool]
	Weight                   reactive.Variable[uint64]
	AttestedWeight           reactive.Variable[uint64]
	CumulativeAttestedWeight reactive.Variable[uint64]
	IsSolid                  reactive.Event
	IsAttested               reactive.Event
	IsVerified               reactive.Event
	IsRoot                   reactive.Event
	IsEvicted                reactive.Event

	protocol                                *Protocol
	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
	isDirectlyAboveLatestVerifiedCommitment reactive.Variable[bool]
	isBelowSyncThreshold                    reactive.Event
	isBelowWarpSyncThreshold                reactive.Event

	log.Logger
}

func NewCommitment(commitment *model.Commitment, protocol *Protocol) *Commitment {
	c := &Commitment{
		Commitment: commitment,

		Parent:                   reactive.NewVariable[*Commitment](),
		MainChild:                reactive.NewVariable[*Commitment](),
		Children:                 reactive.NewSet[*Commitment](),
		SpawnedChain:             reactive.NewVariable[*Chain](),
		Chain:                    reactive.NewVariable[*Chain](),
		Engine:                   reactive.NewVariable[*engine.Engine](),
		RequestAttestations:      reactive.NewVariable[bool](),
		RequestBlocks:            reactive.NewVariable[bool](),
		RequestedBlocksReceived:  reactive.NewVariable[bool](),
		Weight:                   reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](func(currentValue uint64, newValue uint64) uint64 { return max(currentValue, newValue) }),
		CumulativeAttestedWeight: reactive.NewVariable[uint64](),
		IsSolid:                  reactive.NewEvent(),
		IsAttested:               reactive.NewEvent(),
		IsVerified:               reactive.NewEvent(),
		IsRoot:                   reactive.NewEvent(),
		IsEvicted:                reactive.NewEvent(),

		protocol:                                protocol,
		isDirectlyAboveLatestAttestedCommitment: reactive.NewVariable[bool](),
		isDirectlyAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		isBelowSyncThreshold:                    reactive.NewEvent(),
		isBelowWarpSyncThreshold:                reactive.NewEvent(),
	}

	c.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		parent.registerChild(c)

		c.Chain.InheritFrom(reactive.NewDerivedVariable2(func(parentChain, spawnedChain *Chain) *Chain {
			if spawnedChain != nil {
				return spawnedChain
			}

			return parentChain
		}, parent.Chain, c.SpawnedChain))

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

		c.isDirectlyAboveLatestVerifiedCommitment.InheritFrom(reactive.NewDerivedVariable2(func(parentIsVerified, isVerified bool) bool {
			return parentIsVerified && !isVerified
		}, parent.IsVerified, c.IsVerified))

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
		if chain == nil {
			return
		}

		withinContext(func() (unsubscribe func()) {
			requestAttestations := reactive.NewDerivedVariable2(func(verifyAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
				return verifyAttestations && isDirectlyAboveLatestAttestedCommitment
			}, chain.VerifyAttestations, c.isDirectlyAboveLatestAttestedCommitment)

			c.RequestAttestations.InheritFrom(requestAttestations)

			return lo.Batch(
				chain.registerCommitment(c),

				c.Engine.InheritFrom(chain.Engine),

				c.RequestBlocks.InheritFrom(reactive.NewDerivedVariable3(func(spawnedEngine *engine.Engine, warpSyncChain, isDirectlyAboveLatestVerifiedCommitment bool) bool {
					return spawnedEngine != nil && warpSyncChain && isDirectlyAboveLatestVerifiedCommitment
				}, chain.SpawnedEngine, chain.WarpSync, c.isDirectlyAboveLatestVerifiedCommitment)),

				requestAttestations.Unsubscribe,
			)
		})
	})

	c.IsRoot.OnTrigger(func() {
		c.IsSolid.Set(true)
		c.IsAttested.Set(true)
		c.IsVerified.Set(true)
		c.isBelowWarpSyncThreshold.Set(true)
		c.isBelowSyncThreshold.Set(true)
	})

	c.Logger = protocol.NewEntityLogger(fmt.Sprintf("Slot%d.", commitment.Slot()), c.IsEvicted, func(entityLogger log.Logger) {
		c.isDirectlyAboveLatestVerifiedCommitment.LogUpdates(entityLogger, log.LevelTrace, "isDirectlyAboveLatestVerifiedCommitment")
		c.RequestBlocks.LogUpdates(entityLogger, log.LevelTrace, "RequestBlocks")
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
			if c.IsRoot.WasTriggered() {
				return spawnedChain
			}

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

					spawnedChain = NewChain(c.protocol)

					c.protocol.LogDebug("new chain created", "name", spawnedChain.LogName(), "forkingPoint", c.LogName())

					spawnedChain.ForkingPoint.Set(c)
				}

				return spawnedChain
			}
		})
	}
}

func (c *Commitment) promote(targetChain *Chain) {
	if currentChain := c.Chain.Get(); currentChain != targetChain {
		if currentChain == nil {
			// since we only promote commitments that come from an engine, this can only happen if the commitment is the
			// root commitment of the main chain that is the first commitment ever published (which means that we can just
			// set the chain that we want it to have)
			c.Chain.Set(targetChain)
			c.SpawnedChain.Set(targetChain)
		} else if parent := c.Parent.Get(); parent.Chain.Get() == targetChain {
			parent.MainChild.Set(c)
		}
	}
}

func (c *Commitment) triggerEventIfBelowThreshold(event func(*Commitment) reactive.Event, chainThreshold func(*Chain) reactive.Variable[iotago.SlotIndex]) {
	c.Chain.OnUpdateWithContext(func(_, chain *Chain, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		if chain == nil {
			return
		}

		// only monitor the threshold after the parent event was triggered (minimize listeners to same threshold)
		withinContext(func() (unsubscribe func()) {
			return event(c.Parent.Get()).OnTrigger(func() {
				if chain == nil {
					c.LogError("chain is nil IN HERE")
				}

				// since events only trigger once, we unsubscribe from the threshold after the trigger condition is met
				chainThreshold(chain).OnUpdateOnce(func(_, _ iotago.SlotIndex) {
					event(c).Trigger()
				}, func(_, slot iotago.SlotIndex) bool {
					return c.Slot() < slot
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
