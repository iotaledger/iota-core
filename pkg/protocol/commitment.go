package protocol

import (
	"bytes"
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

	// CumulativeWeight contains the cumulative weight of all Commitments up to this point.
	CumulativeWeight reactive.Variable[uint64]

	// CumulativeAttestedWeight contains the cumulative weight of all attested Commitments up to this point.
	CumulativeAttestedWeight reactive.Variable[uint64]

	// CumulativeVerifiedWeight contains the cumulative weight of all verified Commitments up to this point.
	CumulativeVerifiedWeight reactive.Variable[uint64]

	// IsRoot contains a flag indicating if this Commitment is the root of the Chain.
	IsRoot reactive.Event

	// IsAttested contains a flag indicating if we have received attestations for this Commitment.
	IsAttested reactive.Event

	// IsSynced contains a flag that indicates if a Commitment was fully downloaded and processed.
	IsSynced reactive.Event

	// IsCommittable contains a flag that indicates if a Commitment is ready to be committed by the warp sync process.
	IsCommittable reactive.Event

	// IsVerified contains a flag indicating if this Commitment is verified (we produced this Commitment ourselves by
	// booking all the contained blocks and transactions).
	IsVerified reactive.Event

	// IsAboveLatestVerifiedCommitment contains a flag indicating if this Commitment is above the latest verified
	// Commitment.
	IsAboveLatestVerifiedCommitment reactive.Variable[bool]

	// ReplayDroppedBlocks contains a flag indicating if we should replay the blocks that were dropped while the
	// Commitment was pending.
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
		AttestedWeight:                  reactive.NewVariable[uint64](func(currentValue uint64, newValue uint64) uint64 { return max(currentValue, newValue) }), //nolint:gocritic // easier to read
		CumulativeWeight:                reactive.NewVariable[uint64](),
		CumulativeAttestedWeight:        reactive.NewVariable[uint64](),
		CumulativeVerifiedWeight:        reactive.NewVariable[uint64](),
		IsRoot:                          reactive.NewEvent(),
		IsAttested:                      reactive.NewEvent(),
		IsSynced:                        reactive.NewEvent(),
		IsCommittable:                   reactive.NewEvent(),
		IsVerified:                      reactive.NewEvent(),
		IsAboveLatestVerifiedCommitment: reactive.NewVariable[bool](),
		ReplayDroppedBlocks:             reactive.NewVariable[bool](),
		IsEvicted:                       reactive.NewEvent(),
		commitments:                     commitments,
	}

	shutdown := lo.BatchReverse(
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

// Less is a function that is used to break ties between two Commitments that have the same cumulative weight by using
// the ID of their divergence points (the first commitment that is different between their chains).
func (c *Commitment) Less(other *Commitment) bool {
	// trivial case where both commitments are the same or one of them is nil
	switch {
	case c == other:
		return false
	case c == nil:
		return true
	case other == nil:
		return false
	}

	// trivial case where both commitments have the same chain
	largerChain, smallerChain := other.Chain.Get(), c.Chain.Get()
	if largerChain == smallerChain {
		return false
	}

	// iterate until we find the divergence point of both chains
	for {
		// trivial case where one of the chains is nil
		if largerChain == nil {
			return false
		} else if smallerChain == nil {
			return true
		}

		// trivial case where one of the chains has no forking point, yet
		forkingPointOfLargerChain, forkingPointOfSmallerChain := largerChain.ForkingPoint.Get(), smallerChain.ForkingPoint.Get()
		if forkingPointOfLargerChain == nil {
			return false
		} else if forkingPointOfSmallerChain == nil {
			return true
		}

		// if the forking points of both chains have the same parent, then the forking points are the divergence points
		if forkingPointOfLargerChain.Slot() == forkingPointOfSmallerChain.Slot() && forkingPointOfLargerChain.Parent.Get() == forkingPointOfSmallerChain.Parent.Get() {
			return bytes.Compare(lo.PanicOnErr(forkingPointOfLargerChain.ID().Bytes()), lo.PanicOnErr(forkingPointOfSmallerChain.ID().Bytes())) > 0
		}

		// iterate by traversing the parent of the chain with the higher forking point first
		if forkingPointOfLargerChain.Slot() > forkingPointOfSmallerChain.Slot() {
			// iterate to parent
			largerChain = largerChain.ParentChain.Get()

			// terminate if we reach a common chain
			if largerChain == smallerChain {
				divergencePointB, divergencePointBExists := smallerChain.Commitment(forkingPointOfLargerChain.Slot())

				return !divergencePointBExists || bytes.Compare(lo.PanicOnErr(forkingPointOfLargerChain.ID().Bytes()), lo.PanicOnErr(divergencePointB.ID().Bytes())) > 0
			}
		} else {
			// iterate to parent
			smallerChain = smallerChain.ParentChain.Get()

			// terminate if we reach a common chain
			if smallerChain == largerChain {
				divergencePointA, divergencePointAExists := largerChain.Commitment(forkingPointOfSmallerChain.Slot())

				return divergencePointAExists && bytes.Compare(lo.PanicOnErr(divergencePointA.ID().Bytes()), lo.PanicOnErr(forkingPointOfSmallerChain.ID().Bytes())) > 0
			}
		}
	}
}

// initLogger initializes the Logger of this Commitment.
func (c *Commitment) initLogger() (shutdown func()) {
	c.Logger = c.commitments.NewChildLogger(fmt.Sprintf("Slot%d.", c.Slot()), true)

	return lo.BatchReverse(
		c.Parent.LogUpdates(c, log.LevelTrace, "Parent", (*Commitment).LogName),
		c.MainChild.LogUpdates(c, log.LevelTrace, "MainChild", (*Commitment).LogName),
		c.Chain.LogUpdates(c, log.LevelTrace, "Chain", (*Chain).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.WarpSyncBlocks.LogUpdates(c, log.LevelTrace, "WarpSyncBlocks"),
		c.Weight.LogUpdates(c, log.LevelTrace, "Weight"),
		c.AttestedWeight.LogUpdates(c, log.LevelTrace, "AttestedWeight"),
		c.CumulativeWeight.LogUpdates(c, log.LevelTrace, "CumulativeWeight"),
		c.CumulativeAttestedWeight.LogUpdates(c, log.LevelTrace, "CumulativeAttestedWeight"),
		c.CumulativeVerifiedWeight.LogUpdates(c, log.LevelTrace, "CumulativeVerifiedWeight"),
		c.IsRoot.LogUpdates(c, log.LevelTrace, "IsRoot"),
		c.IsAttested.LogUpdates(c, log.LevelTrace, "IsAttested"),
		c.IsSynced.LogUpdates(c, log.LevelTrace, "IsSynced"),
		c.IsCommittable.LogUpdates(c, log.LevelTrace, "IsCommittable"),
		c.IsVerified.LogUpdates(c, log.LevelTrace, "IsVerified"),
		c.ReplayDroppedBlocks.LogUpdates(c, log.LevelTrace, "ReplayDroppedBlocks"),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		c.Logger.UnsubscribeFromParentLogger,
	)
}

// initDerivedProperties initializes the behavior of this Commitment by setting up the relations between its properties.
func (c *Commitment) initDerivedProperties() (shutdown func()) {
	return lo.BatchReverse(
		// mark commitments that are marked as root as verified
		c.IsVerified.InheritFrom(c.IsRoot),

		// mark commitments that are marked as verified as attested and synced
		c.IsAttested.InheritFrom(c.IsVerified),
		c.IsSynced.InheritFrom(c.IsVerified),

		c.deriveCumulativeVerifiedWeight(),

		c.Parent.WithNonEmptyValue(func(parent *Commitment) func() {
			if parent.Commitment.CumulativeWeight() <= c.Commitment.CumulativeWeight() { // prevent overflow in uint64
				c.Weight.Set(c.Commitment.CumulativeWeight() - parent.Commitment.CumulativeWeight())
				c.CumulativeWeight.Set(c.Commitment.CumulativeWeight())
			}

			parent.registerChild(c)

			return lo.BatchReverse(
				c.deriveChain(parent),

				c.deriveCumulativeAttestedWeight(parent),
				c.deriveIsAboveLatestVerifiedCommitment(parent),

				c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
					return lo.BatchReverse(
						c.deriveRequestAttestations(chain, parent),

						// only start requesting blocks once the engine is ready
						chain.WithInitializedEngine(func(_ *engine.Engine) (shutdown func()) {
							return c.deriveWarpSyncBlocks(chain, parent)
						}),
					)
				}),
			)
		}),

		c.Chain.WithNonEmptyValue(func(chain *Chain) func() {
			return lo.BatchReverse(
				chain.addCommitment(c),

				c.deriveReplayDroppedBlocks(chain),
			)
		}),
	)
}

// registerChild adds the given Commitment as a child of this Commitment and sets it as the main child if it is the
// first child of this Commitment.
func (c *Commitment) registerChild(child *Commitment) {
	c.MainChild.Compute(func(mainChild *Commitment) *Commitment {
		if !c.Children.Add(child) || mainChild != nil {
			return mainChild
		}

		return child
	})
}

// deriveChain derives the Chain of this Commitment which is either inherited from the parent if we are the main child
// or a newly created chain.
func (c *Commitment) deriveChain(parent *Commitment) func() {
	return c.Chain.DeriveValueFrom(reactive.NewDerivedVariable3(func(currentChain *Chain, isRoot bool, mainChild *Commitment, parentChain *Chain) *Chain {
		// do not adjust the chain of the root commitment (it is set from the outside)
		if isRoot {
			return currentChain
		}

		// If we are not the main child of our parent, we spawn a new chain.
		// Here we basically move commitments to a new chain if there's a fork.
		if c != mainChild {
			if currentChain == nil || currentChain == parentChain {
				currentChain = c.commitments.protocol.Chains.newChain()
				currentChain.ForkingPoint.Set(c)
			}

			return currentChain
		}

		// If we are the main child of our parent, and our chain is not the parent chain,
		// then we inherit the parent chain and evict the current one.
		// We will spawn a new one if we ever change back to not being the main child.
		// Here we basically move commitments to the parent chain.
		if currentChain != nil && currentChain != parentChain {
			// TODO: refactor it to use a dedicated WorkerPool
			go currentChain.IsEvicted.Trigger()
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

// deriveCumulativeVerifiedWeight derives the CumulativeVerifiedWeight of this Commitment which is the same as the
// CumulativeWeight of the underlying model.Commitment if this Commitment is verified.
func (c *Commitment) deriveCumulativeVerifiedWeight() func() {
	return c.IsVerified.OnTrigger(func() {
		c.CumulativeVerifiedWeight.Set(c.Commitment.CumulativeWeight())
	})
}

// deriveIsAboveLatestVerifiedCommitment derives the IsAboveLatestVerifiedCommitment flag of this Commitment which is
// true if the parent is already above the latest verified Commitment or if the parent is verified and we are not.
func (c *Commitment) deriveIsAboveLatestVerifiedCommitment(parent *Commitment) func() {
	return c.IsAboveLatestVerifiedCommitment.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, parentAboveLatestVerifiedCommitment bool, parentIsVerified bool, isVerified bool) bool {
		return parentAboveLatestVerifiedCommitment || (parentIsVerified && !isVerified)
	}, parent.IsAboveLatestVerifiedCommitment, parent.IsVerified, c.IsVerified))
}

// deriveRequestAttestations derives the RequestAttestations flag of this Commitment which is true if our Chain is
// requesting attestations (while not having an engine), and we are the directly above the latest attested Commitment.
func (c *Commitment) deriveRequestAttestations(chain *Chain, parent *Commitment) func() {
	return c.RequestAttestations.DeriveValueFrom(reactive.NewDerivedVariable4(func(_ bool, startEngine bool, verifyAttestations bool, parentIsAttested bool, isAttested bool) bool {
		return !startEngine && verifyAttestations && parentIsAttested && !isAttested
	}, chain.StartEngine, chain.RequestAttestations, parent.IsAttested, c.IsAttested))
}

// deriveWarpSyncBlocks derives the WarpSyncBlocks flag of this Commitment which is true if our Chain is requesting
// warp sync, and we are the directly above the latest commitment that is synced (has downloaded everything).
func (c *Commitment) deriveWarpSyncBlocks(chain *Chain, parent *Commitment) func() {
	return c.WarpSyncBlocks.DeriveValueFrom(reactive.NewDerivedVariable3(func(_ bool, warpSyncMode bool, parentIsSynced bool, isSynced bool) bool {
		return warpSyncMode && parentIsSynced && !isSynced
	}, chain.WarpSyncMode, parent.IsSynced, c.IsSynced))
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
		if parent := c.Parent.Get(); parent != nil && parent.Chain.Get() == targetChain {
			parent.MainChild.Set(c)
		}
	}
}

// cumulativeWeight returns the Variable that contains the cumulative weight of this Commitment.
func (c *Commitment) cumulativeWeight() reactive.Variable[uint64] {
	return c.CumulativeWeight
}

// cumulativeAttestedWeight returns the Variable that contains the cumulative attested weight of this Commitment.
func (c *Commitment) cumulativeAttestedWeight() reactive.Variable[uint64] {
	return c.CumulativeAttestedWeight
}

// cumulativeVerifiedWeight returns the Variable that contains the cumulative verified weight of this Commitment.
func (c *Commitment) cumulativeVerifiedWeight() reactive.Variable[uint64] {
	return c.CumulativeVerifiedWeight
}
