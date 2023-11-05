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
		IsRoot:                          reactive.NewEvent(),
		IsSolid:                         reactive.NewEvent(),
		IsAttested:                      reactive.NewEvent(),
		IsVerified:                      reactive.NewEvent(),
		IsAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		InSyncRange:                     reactive.NewVariable[bool](),
		IsEvicted:                       reactive.NewEvent(),

		protocol: protocol,
	}

	c.Logger = protocol.NewEntityLogger(fmt.Sprintf("Slot%d.", commitment.Slot()), c.IsEvicted, func(entityLogger log.Logger) {
		c.Parent.LogUpdates(entityLogger, log.LevelTrace, "Parent", (*Commitment).LogName)
		// Children
		c.MainChild.LogUpdates(entityLogger, log.LevelTrace, "MainChild", (*Commitment).LogName)
		c.SpawnedChain.LogUpdates(entityLogger, log.LevelTrace, "SpawnedChain", (*Chain).LogName)
		c.Chain.LogUpdates(entityLogger, log.LevelTrace, "Chain", (*Chain).LogName)
		c.RequestAttestations.LogUpdates(entityLogger, log.LevelTrace, "RequestAttestations")
		c.WarpSync.LogUpdates(entityLogger, log.LevelTrace, "WarpSync")
		c.IsSolid.LogUpdates(entityLogger, log.LevelTrace, "IsSolid")
		c.IsVerified.LogUpdates(entityLogger, log.LevelTrace, "IsVerified")
		c.IsAttested.LogUpdates(entityLogger, log.LevelTrace, "IsAttested")
		c.InSyncRange.LogUpdates(entityLogger, log.LevelTrace, "InSyncRange")
		c.Weight.LogUpdates(entityLogger, log.LevelTrace, "Weight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.CumulativeAttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "CumulativeAttestedWeight")
		c.IsRoot.LogUpdates(entityLogger, log.LevelTrace, "IsRoot")
	})

	var spawnedChain *Chain

	unsubscribe := lo.Batch(
		// populate the local copy of the spawned chain variable with the first value we ever set
		c.SpawnedChain.OnUpdateOnce(func(_, newSpawnedChain *Chain) { spawnedChain = newSpawnedChain }),

		c.IsSolid.InheritFrom(c.IsRoot),

		c.IsAttested.InheritFrom(c.IsRoot),

		c.IsVerified.InheritFrom(c.IsRoot),

		c.Parent.WithNonEmptyValue(func(parent *Commitment) func() {
			c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

			// TODO: REMOVE ON UNSUBSCRIBE
			parent.MainChild.Compute(func(mainChild *Commitment) *Commitment {
				return lo.Cond(mainChild != nil, mainChild, c)
			})

			return lo.Batch(
				c.SpawnedChain.DeriveValueFrom(reactive.NewDerivedVariable2(func(isRoot bool, mainChild *Commitment) *Chain {
					if !isRoot { // do not automatically adjust the chain of the root commitment
						if mainChild != c {
							if spawnedChain == nil {
								spawnedChain = NewChain(protocol)
								spawnedChain.ForkingPoint.Set(c)
							}
						} else {
							if spawnedChain != nil {
								spawnedChain.IsEvicted.Trigger()
								spawnedChain = nil
							}
						}
					}

					return spawnedChain
				}, c.IsRoot, parent.MainChild)),

				c.Chain.DeriveValueFrom(reactive.NewDerivedVariable2(func(parentChain, spawnedChain *Chain) *Chain {
					return lo.Cond(spawnedChain != nil, spawnedChain, parentChain)
				}, parent.Chain, c.SpawnedChain)),

				c.CumulativeAttestedWeight.DeriveValueFrom(reactive.NewDerivedVariable2(func(parentCumulativeAttestedWeight, attestedWeight uint64) uint64 {
					return parentCumulativeAttestedWeight + attestedWeight
				}, parent.CumulativeAttestedWeight, c.AttestedWeight)),

				c.IsAboveLatestVerifiedCommitment.DeriveValueFrom(reactive.NewDerivedVariable3(func(parentAboveLatestVerifiedCommitment, parentIsVerified, isVerified bool) bool {
					return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
				}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified)),

				c.IsSolid.InheritFrom(parent.IsSolid),

				c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
					return lo.Batch(
						c.InSyncRange.DeriveValueFrom(reactive.NewDerivedVariable3(func(spawnedEngine *engine.Engine, warpSyncing, isAboveLatestVerifiedCommitment bool) bool {
							return spawnedEngine != nil && !warpSyncing && isAboveLatestVerifiedCommitment
						}, chain.SpawnedEngine, chain.WarpSync, c.IsAboveLatestVerifiedCommitment)),

						c.WarpSync.DeriveValueFrom(reactive.NewDerivedVariable4(func(spawnedEngine *engine.Engine, warpSync, parentIsVerified, isVerified bool) bool {
							return spawnedEngine != nil && warpSync && parentIsVerified && !isVerified
						}, chain.SpawnedEngine, chain.WarpSync, parent.IsVerified, c.IsVerified)),

						c.RequestAttestations.DeriveValueFrom(reactive.NewDerivedVariable3(func(verifyAttestations, parentIsAttested, isAttested bool) bool {
							return verifyAttestations && parentIsAttested && !isAttested
						}, chain.VerifyAttestations, parent.IsAttested, c.IsAttested)),
					)
				}),
			)
		}),

		c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
			return chain.registerCommitment(c)
		}),
	)

	c.IsEvicted.OnTrigger(unsubscribe)

	return c
}

func (c *Commitment) Engine() *engine.Engine {
	if chain := c.Chain.Get(); chain != nil {
		return chain.Engine.Get()
	}

	return nil
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
