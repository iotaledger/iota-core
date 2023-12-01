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

	// IsAttested contains a flag indicating if we have received attestations for this Commitment.
	IsAttested reactive.Event

	// IsFullyBooked contains a flag indicating if we have received all blocks for this Commitment.
	IsFullyBooked reactive.Event

	// IsCommittable contains a flag indicating if this Commitment is committable (we have received all blocks and all attestations).
	IsCommittable reactive.Event

	// IsCommitted contains a flag indicating if we Commitment produced this Commitment ourselves by replaying all the
	// blocks of the Commitment.
	IsCommitted reactive.Event

	// IsAboveLatestVerifiedCommitment contains a flag indicating if this Commitment is above the latest verified
	// Commitment.
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]

	// ReplayDroppedBlocks contains a flag indicating if we should replay the blocks that were dropped while the
	//Commitment was pending.
	ReplayDroppedBlocks reactive.Variable[bool]

	// IsEvicted contains a flag indicating if this Commitment was evicted from the Protocol.
	IsEvicted reactive.Event

	// commitments contains a reference to the Commitments instance that this Commitment belongs to.
	commitments *Commitments

	// Logger embeds a logger that can be used to log messages emitted by this Commitment.
	log.Logger
}

// NewCommitment creates a new Commitment from the given model.Commitment.
func newCommitment(commitments *Commitments, model *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment:                      model,
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
		IsAttested:                      reactive.NewEvent(),
		IsFullyBooked:                   reactive.NewEvent(),
		IsCommittable:                   reactive.NewEvent(),
		IsCommitted:                     reactive.NewEvent(),
		IsAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		ReplayDroppedBlocks:             reactive.NewVariable[bool](),
		IsEvicted:                       reactive.NewEvent(),
		commitments:                     commitments,
	}

	shutdown := lo.Batch(
		c.initLogger(),
		c.initDerivedProperties(),
	)

	c.IsEvicted.OnTrigger(shutdown)

	return c
}

// TargetEngine returns the engine that is responsible for booking the blocks of this Commitment.
func (c *Commitment) TargetEngine() *engine.Engine {
	if chain := c.Chain.Get(); chain != nil {
		return chain.Engine.Get()
	}

	return nil
}

// initLogger initializes the Logger of this Commitment.
func (c *Commitment) initLogger() (shutdown func()) {
	c.Logger, shutdown = c.commitments.NewEntityLogger(fmt.Sprintf("Slot%d.", c.Slot()))

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
		c.IsAttested.LogUpdates(c, log.LevelTrace, "IsAttested"),
		c.IsFullyBooked.LogUpdates(c, log.LevelTrace, "IsFullyBooked"),
		c.IsCommittable.LogUpdates(c, log.LevelTrace, "IsCommittable"),
		c.IsCommitted.LogUpdates(c, log.LevelTrace, "IsCommitted"),
		c.ReplayDroppedBlocks.LogUpdates(c, log.LevelTrace, "ReplayDroppedBlocks"),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdown,
	)
}

// initDerivedProperties initializes the behavior of this Commitment by setting up the relations between its properties.
func (c *Commitment) initDerivedProperties() (shutdown func()) {
	return lo.Batch(
		// mark commitments that are marked as root as verified
		c.IsCommitted.InheritFrom(c.IsRoot),
		c.IsAboveLatestVerifiedCommitment.InheritFrom(c.IsRoot),

		// mark commitments that are marked as verified as attested, fully booked and committable
		c.IsAttested.InheritFrom(c.IsCommitted),
		c.IsFullyBooked.InheritFrom(c.IsCommitted),
		c.IsCommittable.InheritFrom(c.IsCommitted),

		c.Parent.WithNonEmptyValue(func(parent *Commitment) func() {
			// the weight can be fixed as a one time operation (as it only relies on static information from the parent
			// commitment)
			c.Weight.Set(c.CumulativeWeight() - parent.CumulativeWeight())

			return lo.Batch(
				parent.deriveChildren(c),

				c.deriveChain(parent),
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

// deriveChain derives the Chain of this Commitment which is either inherited from the parent if we are the main child
// or a newly created chain.
func (c *Commitment) deriveChain(parent *Commitment) func() {
	return c.Chain.DeriveValueFrom(reactive.NewDerivedVariable3(func(currentChain *Chain, isRoot bool, mainChild *Commitment, parentChain *Chain) *Chain {
		// do not adjust the chain of the root commitment (it is set from the outside)
		if isRoot {
			return currentChain
		}

		// if we are not the main child of our parent, we spawn a new chain
		if c != mainChild {
			if currentChain == nil {
				currentChain = c.commitments.protocol.Chains.newChain()
				currentChain.ForkingPoint.Set(c)
				currentChain.LatestProducedCommitment.Set(parent)
			}

			return currentChain
		}

		// if we are the main child of our parent, and our chain is not the parent chain (that we are supposed to
		// inherit), then we evict our current chain (we will spawn a new one if we ever change back to not being the
		// main child)
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
	return c.IsAboveLatestVerifiedCommitment.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, parentAboveLatestVerifiedCommitment bool, parentIsCommitted bool, isCommitted bool) bool {
		return parentAboveLatestVerifiedCommitment || (parentIsCommitted && !isCommitted)
	}, parent.IsAboveLatestVerifiedCommitment, parent.IsCommitted, c.IsCommitted))
}

// deriveRequestAttestations derives the RequestAttestations flag of this Commitment which is true if our Chain is
// requesting attestations (while not having an engine), and we are the directly above the latest attested Commitment.
func (c *Commitment) deriveRequestAttestations(chain *Chain, parent *Commitment) func() {
	return c.RequestAttestations.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, startEngine bool, verifyAttestations bool, parentIsAttested bool, isAttested bool) bool {
		return !startEngine && verifyAttestations && parentIsAttested && !isAttested
	}, chain.StartEngine, chain.RequestAttestations, parent.IsAttested, c.IsAttested))
}

// deriveWarpSyncBlocks derives the WarpSyncBlocks flag of this Commitment which is true if our Chain is requesting
// warp sync, and we are the directly above the latest verified Commitment.
func (c *Commitment) deriveWarpSyncBlocks(chain *Chain, parent *Commitment) func() {
	return c.WarpSyncBlocks.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, engineInstance *engine.Engine, warpSync bool, parentIsFullyBooked bool, isFullyBooked bool) bool {
		return engineInstance != nil && warpSync && parentIsFullyBooked && !isFullyBooked
	}, chain.Engine, chain.WarpSyncMode, parent.IsFullyBooked, c.IsFullyBooked))
}

// deriveReplayDroppedBlocks derives the ReplayDroppedBlocks flag of this Commitment which is true if our Chain has an
// engine, is no longer requesting warp sync, and we are above the latest verified Commitment.
func (c *Commitment) deriveReplayDroppedBlocks(chain *Chain) func() {
	return c.ReplayDroppedBlocks.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, engineInstance *engine.Engine, warpSyncing bool, isAboveLatestVerifiedCommitment bool) bool {
		return engineInstance != nil && !warpSyncing && isAboveLatestVerifiedCommitment
	}, chain.Engine, chain.WarpSyncMode, c.IsAboveLatestVerifiedCommitment))
}

// forceChain forces the Chain of this Commitment to the given Chain by promoting it to the main child of its parent if
// the parent is on the target Chain.
func (c *Commitment) forceChain(targetChain *Chain) {
	if currentChain := c.Chain.Get(); currentChain != targetChain {
		if parent := c.Parent.Get(); parent.Chain.Get() == targetChain {
			parent.MainChild.Set(c)
		}
	}
}
