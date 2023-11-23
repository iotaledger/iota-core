package protocol

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	Parent                          reactive.Variable[*Commitment]
	Children                        reactive.Set[*Commitment]
	MainChild                       reactive.Variable[*Commitment]
	Chain                           reactive.Variable[*Chain]
	RequestAttestations             reactive.Variable[bool]
	RequestBlocksToWarpSync         reactive.Variable[bool]
	BlocksToWarpSync                reactive.Variable[ds.Set[iotago.BlockID]]
	Weight                          reactive.Variable[uint64]
	AttestedWeight                  reactive.Variable[uint64]
	CumulativeAttestedWeight        reactive.Variable[uint64]
	IsRoot                          reactive.Event
	IsSolid                         reactive.Event
	IsAttested                      reactive.Event
	IsVerified                      reactive.Event
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]
	ReplayDroppedBlocks             reactive.Variable[bool]
	IsEvicted                       reactive.Event

	*model.Commitment
	log.Logger
}

func NewCommitment(commitment *model.Commitment, chains *Chains) *Commitment {
	return (&Commitment{
		Commitment:                      commitment,
		Parent:                          reactive.NewVariable[*Commitment](),
		Children:                        reactive.NewSet[*Commitment](),
		MainChild:                       reactive.NewVariable[*Commitment](),
		Chain:                           reactive.NewVariable[*Chain](),
		RequestAttestations:             reactive.NewVariable[bool](),
		RequestBlocksToWarpSync:         reactive.NewVariable[bool](),
		BlocksToWarpSync:                reactive.NewVariable[ds.Set[iotago.BlockID]](),
		Weight:                          reactive.NewVariable[uint64](),
		AttestedWeight:                  reactive.NewVariable[uint64](func(currentValue uint64, newValue uint64) uint64 { return max(currentValue, newValue) }),
		CumulativeAttestedWeight:        reactive.NewVariable[uint64](),
		IsRoot:                          reactive.NewEvent(),
		IsSolid:                         reactive.NewEvent(),
		IsAttested:                      reactive.NewEvent(),
		IsVerified:                      reactive.NewEvent(),
		IsAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		ReplayDroppedBlocks:             reactive.NewVariable[bool](),
		IsEvicted:                       reactive.NewEvent(),
	}).initLogging(chains).initBehavior(chains)
}

func (c *Commitment) SpawnedEngine() *engine.Engine {
	if chain := c.Chain.Get(); chain != nil {
		return chain.SpawnedEngine.Get()
	}

	return nil
}

func (c *Commitment) initLogging(chains *Chains) (self *Commitment) {
	var shutdownLogger func()
	c.Logger, shutdownLogger = chains.NewEntityLogger(fmt.Sprintf("Slot%d.", c.Slot()))

	teardownLogging := lo.Batch(
		c.Parent.LogUpdates(c, log.LevelTrace, "Parent", (*Commitment).LogName),
		// c.Children.LogUpdates(c, log.LevelTrace, "Children", (*Commitment).LogName),
		c.MainChild.LogUpdates(c, log.LevelTrace, "MainChild", (*Commitment).LogName),
		c.Chain.LogUpdates(c, log.LevelTrace, "Chain", (*Chain).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.RequestBlocksToWarpSync.LogUpdates(c, log.LevelTrace, "RequestBlocksToWarpSync"),
		c.Weight.LogUpdates(c, log.LevelTrace, "Weight"),
		c.AttestedWeight.LogUpdates(c, log.LevelTrace, "AttestedWeight"),
		c.CumulativeAttestedWeight.LogUpdates(c, log.LevelTrace, "CumulativeAttestedWeight"),
		c.IsRoot.LogUpdates(c, log.LevelTrace, "IsRoot"),
		c.IsSolid.LogUpdates(c, log.LevelTrace, "IsSolid"),
		c.IsAttested.LogUpdates(c, log.LevelTrace, "IsAttested"),
		c.IsVerified.LogUpdates(c, log.LevelTrace, "IsVerified"),
		c.ReplayDroppedBlocks.LogUpdates(c, log.LevelTrace, "ReplayDroppedBlocks"),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdownLogger,
	)

	c.IsEvicted.OnTrigger(teardownLogging)

	return c
}

func (c *Commitment) initBehavior(chains *Chains) (self *Commitment) {
	teardownBehavior := lo.Batch(
		c.IsSolid.InheritFrom(c.IsRoot),
		c.IsAttested.InheritFrom(c.IsRoot),
		c.IsVerified.InheritFrom(c.IsRoot),

		c.Parent.WithNonEmptyValue(func(parent *Commitment) func() {
			c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

			return lo.Batch(
				parent.registerChild(c),

				c.Chain.DeriveValueFrom(reactive.NewDerivedVariable3(func(chain *Chain, isRoot bool, mainChild *Commitment, parentChain *Chain) *Chain {
					if isRoot {
						return chain
					}

					if c != mainChild {
						if chain == nil {
							chain = chains.Fork(c)
						}

						return chain
					}

					if chain != nil && chain != parentChain {
						chain.IsEvicted.Trigger()
					}

					return parentChain
				}, c.IsRoot, parent.MainChild, parent.Chain, c.Chain.Get())),

				c.CumulativeAttestedWeight.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ uint64, parentCumulativeAttestedWeight uint64, attestedWeight uint64) uint64 {
					return parentCumulativeAttestedWeight + attestedWeight
				}, parent.CumulativeAttestedWeight, c.AttestedWeight)),

				c.IsAboveLatestVerifiedCommitment.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, parentAboveLatestVerifiedCommitment bool, parentIsVerified bool, isVerified bool) bool {
					return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
				}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified)),

				c.IsSolid.InheritFrom(parent.IsSolid),

				c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
					return lo.Batch(
						c.ReplayDroppedBlocks.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, spawnedEngine *engine.Engine, warpSyncing bool, isAboveLatestVerifiedCommitment bool) bool {
							return spawnedEngine != nil && !warpSyncing && isAboveLatestVerifiedCommitment
						}, chain.SpawnedEngine, chain.WarpSyncMode, c.IsAboveLatestVerifiedCommitment)),

						c.RequestBlocksToWarpSync.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, spawnedEngine *engine.Engine, warpSync bool, parentIsVerified bool, isVerified bool) bool {
							return spawnedEngine != nil && warpSync && parentIsVerified && !isVerified
						}, chain.SpawnedEngine, chain.WarpSyncMode, parent.IsVerified, c.IsVerified)),

						c.RequestAttestations.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, verifyAttestations bool, requestBlocks bool, parentIsAttested bool, isAttested bool) bool {
							return verifyAttestations && !requestBlocks && parentIsAttested && !isAttested
						}, chain.RequestAttestations, chain.RequestBlocks, parent.IsAttested, c.IsAttested)),
					)
				}),
			)
		}),

		c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
			return chain.registerCommitment(c)
		}),
	)

	c.IsEvicted.OnTrigger(teardownBehavior)

	return c
}

func (c *Commitment) registerChild(child *Commitment) (unregisterChild func()) {
	c.MainChild.Compute(func(mainChild *Commitment) *Commitment {
		if !c.Children.Add(child) || mainChild != nil {
			return mainChild
		}

		return child
	})

	return func() {
		c.MainChild.Compute(func(mainChild *Commitment) *Commitment {
			if !c.Children.Delete(child) || child != mainChild {
				return mainChild
			}

			return lo.Return1(c.Children.Any())
		})
	}
}

func (c *Commitment) setChain(targetChain *Chain) {
	if currentChain := c.Chain.Get(); currentChain != targetChain {
		if currentChain == nil { // the root commitment doesn't inherit a chain from its parents
			c.Chain.Set(targetChain)
		} else if parent := c.Parent.Get(); parent.Chain.Get() == targetChain {
			parent.MainChild.Set(c)
		}
	}
}
