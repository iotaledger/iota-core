package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/core/rx"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Commitment struct {
	Parent                          reactive.Variable[*Commitment]
	Children                        reactive.Set[*Commitment]
	MainChild                       reactive.Variable[*Commitment]
	SpawnedChain                    reactive.Variable[*Chain]
	Chain                           reactive.Variable[*Chain]
	RequestAttestations             reactive.Variable[bool]
	WarpSync                        reactive.Variable[bool]
	RequestedBlocksReceived         reactive.Variable[bool]
	Weight                          reactive.Variable[uint64]
	AttestedWeight                  reactive.Variable[uint64]
	CumulativeAttestedWeight        reactive.Variable[uint64]
	IsRoot                          reactive.Event
	IsSolid                         reactive.Event
	IsAttested                      reactive.Event
	IsVerified                      reactive.Event
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]
	InSyncRange                     reactive.Variable[bool]
	IsEvicted                       reactive.Event

	protocol *Protocol

	*model.Commitment

	log.Logger
}

func NewCommitment(commitment *model.Commitment, protocol *Protocol) *Commitment {
	c := &Commitment{
		Commitment:                      commitment,
		Parent:                          reactive.NewVariable[*Commitment](),
		Children:                        reactive.NewSet[*Commitment](),
		MainChild:                       reactive.NewVariable[*Commitment](),
		SpawnedChain:                    reactive.NewVariable[*Chain](),
		Chain:                           reactive.NewVariable[*Chain](),
		RequestAttestations:             reactive.NewVariable[bool](),
		WarpSync:                        reactive.NewVariable[bool](),
		RequestedBlocksReceived:         reactive.NewVariable[bool](),
		Weight:                          reactive.NewVariable[uint64](),
		AttestedWeight:                  reactive.NewVariable[uint64](func(currentValue uint64, newValue uint64) uint64 { return max(currentValue, newValue) }),
		CumulativeAttestedWeight:        reactive.NewVariable[uint64](),
		IsSolid:                         reactive.NewEvent(),
		IsAttested:                      reactive.NewEvent(),
		IsVerified:                      reactive.NewEvent(),
		IsRoot:                          reactive.NewEvent(),
		IsEvicted:                       reactive.NewEvent(),
		IsAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		InSyncRange:                     reactive.NewVariable[bool](),

		protocol: protocol,
	}

	c.Logger = protocol.NewEntityLogger(fmt.Sprintf("Slot%d.", commitment.Slot()), c.IsEvicted, func(entityLogger log.Logger) {
		c.Parent.LogUpdates(entityLogger, log.LevelTrace, "Parent", (*Commitment).LogName)
		c.IsSolid.LogUpdates(entityLogger, log.LevelTrace, "IsSolid")
		c.Chain.LogUpdates(entityLogger, log.LevelTrace, "Chain", (*Chain).LogName)
		c.IsVerified.LogUpdates(entityLogger, log.LevelTrace, "IsVerified")
		c.IsAttested.LogUpdates(entityLogger, log.LevelTrace, "IsAttested")
		c.InSyncRange.LogUpdates(entityLogger, log.LevelTrace, "InSyncRange")
		c.WarpSync.LogUpdates(entityLogger, log.LevelTrace, "WarpSync")
		c.Weight.LogUpdates(entityLogger, log.LevelTrace, "Weight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.CumulativeAttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "CumulativeAttestedWeight")
	})

	unsubscribe := lo.Batch(
		c.IsSolid.InheritFrom(c.IsRoot),
		c.IsAttested.InheritFrom(c.IsRoot),
		c.IsVerified.InheritFrom(c.IsRoot),

		rx.WithNonEmptyValue(c.Parent, func(parent *Commitment) func() {
			c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

			parent.registerChild(c)

			return lo.Batch(
				rx.AssignValue[*Chain](c.Chain, reactive.NewDerivedVariable2(func(parentChain, spawnedChain *Chain) *Chain {
					if spawnedChain != nil {
						return spawnedChain
					}

					return parentChain
				}, parent.Chain, c.SpawnedChain)),

				rx.AssignValue[bool](c.IsSolid, reactive.NewDerivedVariable(func(parentIsSolid bool) bool {
					return parentIsSolid
				}, parent.IsSolid)),

				rx.AssignValue[uint64](c.CumulativeAttestedWeight, reactive.NewDerivedVariable2(func(parentCumulativeAttestedWeight, attestedWeight uint64) uint64 {
					return parentCumulativeAttestedWeight + attestedWeight
				}, parent.CumulativeAttestedWeight, c.AttestedWeight)),

				rx.AssignValue[bool](c.IsAboveLatestVerifiedCommitment, reactive.NewDerivedVariable3(func(parentAboveLatestVerifiedCommitment, parentIsVerified, isVerified bool) bool {
					return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
				}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified)),

				rx.WithNonEmptyValue(c.Chain, func(chain *Chain) func() {
					return lo.Batch(
						rx.AssignValue[bool](c.InSyncRange, reactive.NewDerivedVariable3(func(spawnedEngine *engine.Engine, warpSyncing, isAboveLatestVerifiedCommitment bool) bool {
							return spawnedEngine != nil && !warpSyncing && isAboveLatestVerifiedCommitment
						}, chain.SpawnedEngine, chain.WarpSync, c.IsAboveLatestVerifiedCommitment)),

						rx.AssignValue[bool](c.WarpSync, reactive.NewDerivedVariable4(func(spawnedEngine *engine.Engine, warpSync, parentIsVerified, isVerified bool) bool {
							return spawnedEngine != nil && warpSync && parentIsVerified && !isVerified
						}, chain.SpawnedEngine, chain.WarpSync, parent.IsVerified, c.IsVerified)),

						rx.AssignValue[bool](c.RequestAttestations, reactive.NewDerivedVariable3(func(verifyAttestations, parentIsAttested, isAttested bool) bool {
							return verifyAttestations && parentIsAttested && !isAttested
						}, chain.VerifyAttestations, parent.IsAttested, c.IsAttested)),
					)
				}),
			)
		}),

		rx.WithNonEmptyValue(c.Chain, func(chain *Chain) func() { return chain.registerCommitment(c) }),
	)

	c.IsEvicted.OnTrigger(unsubscribe)

	return c
}

func (c *Commitment) Engine() *engine.Engine {
	chain := c.Chain.Get()
	if chain == nil {
		return nil
	}

	return chain.Engine.Get()
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
