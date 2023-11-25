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

// Commitment represents a commitment to a specific ledger state at a specific point in time that is part of a chain of
// commitments produced by the nodes in the network.
type Commitment struct {
	// Commitment contains the underlying model.Commitment that is represented by this Commitment.
	*model.Commitment

	// Parent contains the Commitment that is referenced as a parent in this Commitment.
	Parent reactive.Variable[*Commitment]

	// Children contains the Commitments that reference this Commitment as a parent.
	Children reactive.Set[*Commitment]

	// MainChild contains the Commitment that is the main child of this Commitment (continues the chain).
	MainChild reactive.Variable[*Commitment]

	// Chain contains the Chain that this Commitment is part of.
	Chain reactive.Variable[*Chain]

	// RequestAttestations contains a flag indicating if the node should request attestations for this Commitment.
	RequestAttestations reactive.Variable[bool]

	// WarpSyncBlocks contains a flag indicating if the node should request the blocks of this Commitment using warp
	// sync.
	WarpSyncBlocks reactive.Variable[bool]

	// BlocksToWarpSync contains the set of blocks that should be requested using warp sync.
	BlocksToWarpSync reactive.Variable[ds.Set[iotago.BlockID]]

	// Weight contains the weight of this Commitment (the difference between the cumulative weight of this Commitment
	// and its parent).
	Weight reactive.Variable[uint64]

	// AttestedWeight contains the weight of the Commitment that was attested by other nodes.
	AttestedWeight reactive.Variable[uint64]

	// CumulativeAttestedWeight contains the cumulative weight of all attested Commitments up to this point.
	CumulativeAttestedWeight reactive.Variable[uint64]

	// IsRoot contains a flag indicating if this Commitment is the root of the Chain.
	IsRoot reactive.Event

	// IsSolid contains a flag indicating if this Commitment is solid (all referenced blocks in its past are known).
	IsSolid reactive.Event

	// IsAttested contains a flag indicating if we have received attestations for this Commitment.
	IsAttested reactive.Event

	// IsVerified contains a flag indicating if this Commitment is verified (we produced this Commitment ourselves by
	// booking all the contained blocks and transactions).
	IsVerified reactive.Event

	// IsAboveLatestVerifiedCommitment contains a flag indicating if this Commitment is above the latest verified
	// Commitment.
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]

	// ReplayDroppedBlocks contains a flag indicating if we should replay the blocks that were dropped while the
	//Commitment was pending.
	ReplayDroppedBlocks reactive.Variable[bool]

	// IsEvicted contains a flag indicating if this Commitment was evicted from the Protocol.
	IsEvicted reactive.Event

	// Logger embeds a logger that can be used to log messages emitted by this Commitment.
	log.Logger
}

// NewCommitment creates a new Commitment from the given model.Commitment.
func newCommitment(commitment *model.Commitment, chains *Chains) *Commitment {
	c := &Commitment{
		Commitment:                      commitment,
		Parent:                          reactive.NewVariable[*Commitment](),
		Children:                        reactive.NewSet[*Commitment](),
		MainChild:                       reactive.NewVariable[*Commitment](),
		Chain:                           reactive.NewVariable[*Chain](),
		RequestAttestations:             reactive.NewVariable[bool](),
		WarpSyncBlocks:                  reactive.NewVariable[bool](),
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
	}

	shutdown := lo.Batch(
		c.initLogger(chains.NewEntityLogger(fmt.Sprintf("Slot%d.", c.Slot()))),
		c.initDerivedProperties(chains),
	)

	c.IsEvicted.OnTrigger(shutdown)

	return c
}

// TargetEngine returns the engine that is responsible for booking the blocks of this Commitment.
func (c *Commitment) TargetEngine() *engine.Engine {
	if chain := c.Chain.Get(); chain != nil {
		return chain.SpawnedEngine.Get()
	}

	return nil
}

// initLogger initializes the Logger of this Commitment.
func (c *Commitment) initLogger(logger log.Logger, shutdownLogger func()) (teardown func()) {
	c.Logger = logger

	return lo.Batch(
		c.Parent.LogUpdates(c, log.LevelTrace, "Parent", (*Commitment).LogName),
		c.MainChild.LogUpdates(c, log.LevelTrace, "MainChild", (*Commitment).LogName),
		c.Chain.LogUpdates(c, log.LevelTrace, "Chain", (*Chain).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.WarpSyncBlocks.LogUpdates(c, log.LevelTrace, "WarpSyncBlocks"),
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
}

// initDerivedProperties initializes the behavior of this Commitment by setting up the relations between its properties.
func (c *Commitment) initDerivedProperties(chains *Chains) (teardown func()) {
	return lo.Batch(
		c.deriveRootProperties(),

		c.Parent.WithNonEmptyValue(func(parent *Commitment) func() {
			// the weight can be fixed as soon as the parent is known (as it only relies on static information from the
			// parent commitment)
			c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

			return lo.Batch(
				parent.deriveChildren(c),

				c.deriveIsSolid(parent),
				c.deriveChain(chains, parent),
				c.deriveCumulativeAttestedWeight(parent),
				c.deriveIsAboveLatestVerifiedCommitment(parent),

				c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
					return lo.Batch(
						c.deriveRequestAttestations(chain, parent),
						c.deriveWarpSyncBlocks(chain, parent),
					)
				}),
			)
		}),

		c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
			return lo.Batch(
				chain.addCommitment(c),

				c.deriveReplayDroppedBlocks(chain),
			)
		}),
	)
}

// deriveRootProperties derives the properties that are supposed to be set for the root Commitment.
func (c *Commitment) deriveRootProperties() (teardown func()) {
	return lo.Batch(
		c.IsSolid.InheritFrom(c.IsRoot),
		c.IsAttested.InheritFrom(c.IsRoot),
		c.IsVerified.InheritFrom(c.IsRoot),
	)
}

// deriveChildren derives the children of this Commitment by adding the given child to the Children set.
func (c *Commitment) deriveChildren(child *Commitment) (unregisterChild func()) {
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

// deriveIsSolid derives the IsSolid flag of this Commitment which is set to true if the parent is known and solid.
func (c *Commitment) deriveIsSolid(parent *Commitment) func() {
	return c.IsSolid.InheritFrom(parent.IsSolid)
}

// deriveChain derives the Chain of this Commitment which is either inherited from the parent if we are the main child
// or a newly created chain.
func (c *Commitment) deriveChain(chains *Chains, parent *Commitment) func() {
	return c.Chain.DeriveValueFrom(reactive.NewDerivedVariable3(func(currentChain *Chain, isRoot bool, mainChild *Commitment, parentChain *Chain) *Chain {
		// do not adjust the chain of the root commitment (it is set from the outside)
		if isRoot {
			return currentChain
		}

		// if we are not the main child of our parent, we create and return a new chain
		if c != mainChild {
			if currentChain == nil {
				currentChain = chains.newChain()
				currentChain.ForkingPoint.Set(c)
			}

			return currentChain
		}

		// if we are the main child of our parent, and we
		if currentChain != nil && currentChain != parentChain {
			currentChain.IsEvicted.Trigger()
		}

		return parentChain
	}, c.IsRoot, parent.MainChild, parent.Chain, c.Chain.Get()))
}

// deriveCumulativeAttestedWeight derives the CumulativeAttestedWeight of this Commitment which is the sum of the
// parent's CumulativeAttestedWeight and the AttestedWeight of this Commitment.
func (c *Commitment) deriveCumulativeAttestedWeight(parent *Commitment) func() {
	return c.CumulativeAttestedWeight.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ uint64, parentCumulativeAttestedWeight uint64, attestedWeight uint64) uint64 {
		return parentCumulativeAttestedWeight + attestedWeight
	}, parent.CumulativeAttestedWeight, c.AttestedWeight))
}

// deriveIsAboveLatestVerifiedCommitment derives the IsAboveLatestVerifiedCommitment flag of this Commitment which is
// true if the parent is already above the latest verified Commitment or if the parent is verified and we are not.
func (c *Commitment) deriveIsAboveLatestVerifiedCommitment(parent *Commitment) func() {
	return c.IsAboveLatestVerifiedCommitment.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, parentAboveLatestVerifiedCommitment bool, parentIsVerified bool, isVerified bool) bool {
		return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
	}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified))
}

// deriveRequestAttestations derives the RequestAttestations flag of this Commitment which is true if our Chain is
// requesting attestations, and we are the directly above the latest attested Commitment.
func (c *Commitment) deriveRequestAttestations(chain *Chain, parent *Commitment) func() {
	return c.RequestAttestations.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, verifyAttestations bool, requestBlocks bool, parentIsAttested bool, isAttested bool) bool {
		return verifyAttestations && !requestBlocks && parentIsAttested && !isAttested
	}, chain.RequestAttestations, chain.RequestBlocks, parent.IsAttested, c.IsAttested))
}

// deriveWarpSyncBlocks derives the WarpSyncBlocks flag of this Commitment which is true if our Chain is requesting
// warp sync, and we are the directly above the latest verified Commitment.
func (c *Commitment) deriveWarpSyncBlocks(chain *Chain, parent *Commitment) func() {
	return c.WarpSyncBlocks.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, spawnedEngine *engine.Engine, warpSync bool, parentIsVerified bool, isVerified bool) bool {
		return spawnedEngine != nil && warpSync && parentIsVerified && !isVerified
	}, chain.SpawnedEngine, chain.WarpSyncMode, parent.IsVerified, c.IsVerified))
}

// deriveReplayDroppedBlocks derives the ReplayDroppedBlocks flag of this Commitment which is true if our Chain has an
// engine, is no longer requesting warp sync, and we are above the latest verified Commitment.
func (c *Commitment) deriveReplayDroppedBlocks(chain *Chain) func() {
	return c.ReplayDroppedBlocks.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, spawnedEngine *engine.Engine, warpSyncing bool, isAboveLatestVerifiedCommitment bool) bool {
		return spawnedEngine != nil && !warpSyncing && isAboveLatestVerifiedCommitment
	}, chain.SpawnedEngine, chain.WarpSyncMode, c.IsAboveLatestVerifiedCommitment))
}

func (c *Commitment) forceChain(targetChain *Chain) {
	if currentChain := c.Chain.Get(); currentChain != targetChain {
		if currentChain == nil { // the root commitment doesn't inherit a chain from its parents
			c.Chain.Set(targetChain)
		} else if parent := c.Parent.Get(); parent.Chain.Get() == targetChain {
			parent.MainChild.Set(c)
		}
	}
}
