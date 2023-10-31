package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
)

type Commitment struct {
	Parent                          reactive.Variable[*Commitment]
	Children                        reactive.Set[*Commitment]
	MainChild                       reactive.Variable[*Commitment]
	SpawnedChain                    reactive.Variable[*Chain]
	Chain                           reactive.Variable[*Chain]
	Engine                          reactive.Variable[*engine.Engine]
	RequestAttestations             reactive.Variable[bool]
	WarpSync                        reactive.Variable[bool]
	RequestedBlocksReceived         reactive.Variable[bool]
	Weight                          reactive.Variable[uint64]
	AttestedWeight                  reactive.Variable[uint64]
	CumulativeAttestedWeight        reactive.Variable[uint64]
	IsSolid                         reactive.Event
	IsAttested                      reactive.Event
	IsVerified                      reactive.Event
	IsRoot                          reactive.Event
	IsEvicted                       reactive.Event
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]
	InSyncRange                     reactive.Variable[bool]

	protocol                                *Protocol
	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]

	*model.Commitment
	log.Logger
}

func NewCommitment(commitment *model.Commitment, protocol *Protocol) *Commitment {
	c := &Commitment{
		Commitment:                      commitment,
		Parent:                          reactive.NewVariable[*Commitment](),
		MainChild:                       reactive.NewVariable[*Commitment](),
		Children:                        reactive.NewSet[*Commitment](),
		SpawnedChain:                    reactive.NewVariable[*Chain](),
		Chain:                           reactive.NewVariable[*Chain](),
		Engine:                          reactive.NewVariable[*engine.Engine](),
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

		protocol:                                protocol,
		isDirectlyAboveLatestAttestedCommitment: reactive.NewVariable[bool](),
	}

	WithDependency(c.Parent,
		DynamicValueWithDependency(c.chain, c.Chain),
		DynamicValueWithDependency(c.isSolid, c.IsSolid),
		DynamicValueWithDependency(c.cumulativeAttestedWeight, c.CumulativeAttestedWeight),
		DynamicValueWithDependency(c.isAboveLatestVerifiedCommitment, c.IsAboveLatestVerifiedCommitment),
		DefinitionWithStaticValue(c.weight, c.Weight),
	)

	WithDependency(c.Chain,
		DynamicValueWithDependency(c.inSyncRange, c.InSyncRange),
	)

	WithDependencies2(c.Parent, c.Chain,
		DynamicValueWith2Dependencies(c.warpSync, c.WarpSync),
	)

	c.Parent.OnUpdateOnce(func(_, parent *Commitment) {
		if parent == nil {
			return
		}

		parent.registerChild(c)

		c.isDirectlyAboveLatestAttestedCommitment.InheritFrom(reactive.NewDerivedVariable2(func(parentIsAttested, isAttested bool) bool {
			return parentIsAttested && !isAttested
		}, parent.IsAttested, c.IsAttested))
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

				requestAttestations.Unsubscribe,
			)
		})
	})

	c.IsRoot.OnTrigger(func() {
		c.IsAttested.Set(true)
		c.IsVerified.Set(true)
		c.IsSolid.Set(true)
	})

	c.Logger = protocol.NewEntityLogger(fmt.Sprintf("Slot%d.", commitment.Slot()), c.IsEvicted, func(entityLogger log.Logger) {
		c.IsSolid.LogUpdates(entityLogger, log.LevelTrace, "IsSolid")
		c.Chain.LogUpdates(entityLogger, log.LevelTrace, "Chain", (*Chain).LogName)
		c.IsVerified.LogUpdates(entityLogger, log.LevelTrace, "IsVerified")
		c.InSyncRange.LogUpdates(entityLogger, log.LevelTrace, "InSyncRange")
		c.WarpSync.LogUpdates(entityLogger, log.LevelTrace, "WarpSync")
		c.Weight.LogUpdates(entityLogger, log.LevelTrace, "Weight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.CumulativeAttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "CumulativeAttestedWeight")
	})

	return c
}

func (c *Commitment) isAboveLatestVerifiedCommitment(parent *Commitment) reactive.DerivedVariable[bool] {
	return reactive.NewDerivedVariable3(func(parentAboveLatestVerifiedCommitment, parentIsVerified, isVerified bool) bool {
		return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
	}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified)
}

func (c *Commitment) inSyncRange(chain *Chain) reactive.DerivedVariable[bool] {
	return reactive.NewDerivedVariable3(func(spawnedEngine *engine.Engine, warpSyncing, isAboveLatestVerifiedCommitment bool) bool {
		return spawnedEngine != nil && !warpSyncing && isAboveLatestVerifiedCommitment
	}, chain.SpawnedEngine, chain.WarpSync, c.IsAboveLatestVerifiedCommitment)
}

func (c *Commitment) chain(parent *Commitment) reactive.DerivedVariable[*Chain] {
	return reactive.NewDerivedVariable2(func(parentChain, spawnedChain *Chain) *Chain {
		if spawnedChain != nil {
			return spawnedChain
		}

		return parentChain
	}, parent.Chain, c.SpawnedChain)
}

func (c *Commitment) cumulativeAttestedWeight(parent *Commitment) reactive.DerivedVariable[uint64] {
	return reactive.NewDerivedVariable3(func(isAttested bool, parentCumulativeAttestedWeight, attestedWeight uint64) uint64 {
		if !isAttested {
			return 0
		}

		return parentCumulativeAttestedWeight + attestedWeight
	}, c.IsAttested, parent.CumulativeAttestedWeight, c.AttestedWeight)
}

func (c *Commitment) isSolid(parent *Commitment) reactive.DerivedVariable[bool] {
	return reactive.NewDerivedVariable2(func(isRoot, parentIsSolid bool) bool {
		return isRoot || parentIsSolid
	}, c.IsRoot, parent.IsSolid)
}

func (c *Commitment) warpSync(parent *Commitment, chain *Chain) reactive.DerivedVariable[bool] {
	return reactive.NewDerivedVariable4(func(spawnedEngine *engine.Engine, warpSync, parentIsVerified, isVerified bool) bool {
		return spawnedEngine != nil && warpSync && parentIsVerified && !isVerified
	}, chain.SpawnedEngine, chain.WarpSync, parent.IsVerified, c.IsVerified)
}

func (c *Commitment) weight(parent *Commitment) uint64 {
	return c.CumulativeWeight() - parent.CumulativeWeight()
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

func WithDependency[S comparable](source reactive.Variable[S], dependencyReceiver ...func(S) func()) (unsubscribe func()) {
	unsubscribeAll := make([]func(), 0)

	return source.OnUpdateWithContext(func(_, parent S, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if parent == *new(S) {
			return
		}

		unsubscribeOnParentUpdate(func() (unsubscribe func()) {
			for _, dependency := range dependencyReceiver {
				if unsubscribeDependency := dependency(parent); unsubscribeDependency != nil {
					unsubscribeAll = append(unsubscribeAll, unsubscribeDependency)
				}
			}

			return lo.Batch(unsubscribeAll...)
		})
	})
}

func WithDependencies2[S1, S2 comparable](source1 reactive.Variable[S1], source2 reactive.Variable[S2], dependencyReceiver ...func(S1, S2) func()) (unsubscribe func()) {
	unsubscribeAll := make([]func(), 0)

	return source1.OnUpdateWithContext(func(_, source1 S1, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if source1 == *new(S1) {
			return
		}

		unsubscribeOnParentUpdate(func() (unsubscribe func()) {
			return source2.OnUpdateWithContext(func(_, source2 S2, unsubscribeOnParentUpdate func(subscriptionFactory func() (unsubscribe func()))) {
				if source2 == *new(S2) {
					return
				}

				unsubscribeOnParentUpdate(func() (unsubscribe func()) {
					for _, dependency := range dependencyReceiver {
						if unsubscribeDependency := dependency(source1, source2); unsubscribeDependency != nil {
							unsubscribeAll = append(unsubscribeAll, unsubscribeDependency)
						}
					}

					return lo.Batch(unsubscribeAll...)
				})
			})
		})
	})
}

func DynamicValueWithDependency[T, S comparable](definition func(S) reactive.DerivedVariable[T], target reactive.Variable[T]) func(parent S) func() {
	return func(parent S) func() {
		derivedVariable := definition(parent)

		return lo.Batch(target.InheritFrom(derivedVariable), derivedVariable.Unsubscribe)
	}
}

func DynamicValueWith2Dependencies[T, S1, S2 comparable](definition func(S1, S2) reactive.DerivedVariable[T], target reactive.Variable[T]) func(S1, S2) func() {
	return func(source1 S1, source2 S2) func() {
		derivedVariable := definition(source1, source2)

		return lo.Batch(target.InheritFrom(derivedVariable), derivedVariable.Unsubscribe)
	}
}

func DefinitionWithStaticValue[T, S comparable](definition func(S) T, target reactive.Variable[T]) func(parent S) func() {
	return func(parent S) func() {
		target.Set(definition(parent))

		return nil
	}
}
