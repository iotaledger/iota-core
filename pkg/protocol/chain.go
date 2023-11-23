package protocol

import (
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chain represents a chain of commitments.
type Chain struct {
	// ForkingPoint contains the first commitment of this chain.
	ForkingPoint reactive.Variable[*Commitment]

	// Parent contains the chain that this chain forked from.
	Parent reactive.Variable[*Chain]

	// Children contains the set of all chains that forked from this chain.
	Children reactive.Set[*Chain]

	// LatestCommitment contains the latest commitment of this chain.
	LatestCommitment reactive.Variable[*Commitment]

	// LatestAttestedCommitment contains the latest commitment of this chain for which attestations were received.
	LatestAttestedCommitment reactive.Variable[*Commitment]

	// LatestProducedCommitment contains the latest commitment of this chain that we produced ourselves by booking the
	// corresponding blocks in the SpawnedEngine.
	LatestProducedCommitment reactive.Variable[*Commitment]

	// ClaimedWeight contains the claimed weight of this chain which is derived from the cumulative weight of the
	// LatestCommitment.
	ClaimedWeight reactive.Variable[uint64]

	// AttestedWeight contains the attested weight of this chain which is derived from the cumulative weight of all
	// attestations up to the LatestAttestedCommitment.
	AttestedWeight reactive.Variable[uint64]

	// VerifiedWeight contains the verified weight of this chain which is derived from the cumulative weight of the
	// latest verified commitment.
	VerifiedWeight reactive.Variable[uint64]

	// WarpSyncMode contains a flag that indicates whether this chain is in warp sync mode.
	WarpSyncMode reactive.Variable[bool]

	// WarpSyncThreshold contains the slot at which the chain will exit warp sync mode which is derived from the latest
	// network slot minus the max committable age.
	WarpSyncThreshold reactive.Variable[iotago.SlotIndex]

	// OutOfSyncThreshold contains the slot at which the chain will consider itself to be out of sync and switch to warp
	// sync mode. It is derived from the latest network slot minus two times the max committable age.
	OutOfSyncThreshold reactive.Variable[iotago.SlotIndex]

	// RequestAttestations contains a flag that indicates whether this chain should verify the claimed weight by
	// requesting attestations.
	RequestAttestations reactive.Variable[bool]

	// RequestBlocks contains a flag that indicates whether this chain should verify the state by requesting blocks and
	// processing them in its SpawnedEngine.
	RequestBlocks reactive.Variable[bool]

	// SpawnedEngine contains the engine that is used to process blocks for this chain.
	SpawnedEngine reactive.Variable[*engine.Engine]

	// IsEvicted contains a flag that indicates whether this chain was evicted.
	IsEvicted reactive.Event

	// commitments contains the commitments that make up this chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// newChain creates a new chain instance.
func newChain(logger log.Logger, shutdownLogger func(), latestSeenSlot reactive.ReadableVariable[iotago.SlotIndex]) *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		Parent:                   reactive.NewVariable[*Chain](),
		Children:                 reactive.NewSet[*Chain](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestProducedCommitment: reactive.NewVariable[*Commitment](),
		ClaimedWeight:            reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](),
		VerifiedWeight:           reactive.NewVariable[uint64](),
		WarpSyncMode:             reactive.NewVariable[bool]().Init(true),
		WarpSyncThreshold:        reactive.NewVariable[iotago.SlotIndex](),
		OutOfSyncThreshold:       reactive.NewVariable[iotago.SlotIndex](),
		RequestAttestations:      reactive.NewVariable[bool](),
		RequestBlocks:            reactive.NewVariable[bool](),
		SpawnedEngine:            reactive.NewVariable[*engine.Engine](),
		IsEvicted:                reactive.NewEvent(),

		commitments: shrinkingmap.New[iotago.SlotIndex, *Commitment](),
	}

	shutdown := lo.Batch(
		c.initLogger(logger, shutdownLogger),
		c.initDerivedProperties(latestSeenSlot),
	)

	c.IsEvicted.OnTrigger(shutdown)

	return c
}

// DispatchBlock dispatches the given block to the chain and its children.
func (c *Chain) DispatchBlock(block *model.Block, src peer.ID) (dispatched bool) {
	// allow to call this method on a nil chain to avoid nil checks in the caller when dispatching to unsolid chains
	if c == nil {
		return false
	}

	// first try to dispatch to our chain
	if spawnedEngine := c.SpawnedEngine.Get(); spawnedEngine != nil {
		// only dispatch blocks that are for slots larger than the latest commitment
		if targetSlot := spawnedEngine.APIForTime(block.ProtocolBlock().Header.IssuingTime).TimeProvider().SlotFromTime(block.ProtocolBlock().Header.IssuingTime); targetSlot > spawnedEngine.LatestCommitment.Get().Slot() {
			// if we are in warp sync mode, then we only accept blocks that are part of the blocks to warp sync.
			if dispatched = !c.WarpSyncMode.Get(); !dispatched {
				if targetCommitment, targetCommitmentExists := c.Commitment(targetSlot); targetCommitmentExists {
					if blocksToWarpSync := targetCommitment.BlocksToWarpSync.Get(); blocksToWarpSync != nil && blocksToWarpSync.Has(block.ID()) {
						dispatched = true
					}
				}
			}

			if dispatched {
				spawnedEngine.ProcessBlockFromPeer(block, src)
			}
		}
	}

	// then try to dispatch to our children
	for _, childChain := range c.Children.ToSlice() {
		dispatched = childChain.DispatchBlock(block, src) || dispatched
	}

	return dispatched
}

// Commitment returns the Commitment for the given slot from the perspective of this chain.
func (c *Chain) Commitment(slot iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c; currentChain != nil; {
		switch forkingPoint := currentChain.ForkingPoint.Get(); {
		case forkingPoint.Slot() == slot:
			return forkingPoint, true
		case slot > forkingPoint.Slot():
			return currentChain.commitments.Get(slot)
		default:
			currentChain = c.Parent.Get()
		}
	}

	return nil, false
}

// LatestEngine returns the latest engine instance that was spawned by the chain itself or one of its ancestors.
func (c *Chain) LatestEngine() *engine.Engine {
	currentChain, currentEngine := c, c.SpawnedEngine.Get()
	for currentEngine == nil {
		if currentChain = c.Parent.Get(); currentChain == nil {
			return nil
		}

		currentEngine = currentChain.SpawnedEngine.Get()
	}

	return currentEngine
}

// initLogger initializes the Logger of this chain.
func (c *Chain) initLogger(logger log.Logger, shutdownLogger func()) (teardown func()) {
	c.Logger = logger

	return lo.Batch(
		c.WarpSyncMode.LogUpdates(c, log.LevelTrace, "WarpSyncMode"),
		c.WarpSyncThreshold.LogUpdates(c, log.LevelTrace, "WarpSyncThreshold"),
		c.OutOfSyncThreshold.LogUpdates(c, log.LevelTrace, "OutOfSyncThreshold"),
		c.ForkingPoint.LogUpdates(c, log.LevelTrace, "ForkingPoint", (*Commitment).LogName),
		c.ClaimedWeight.LogUpdates(c, log.LevelTrace, "ClaimedWeight"),
		c.AttestedWeight.LogUpdates(c, log.LevelTrace, "AttestedWeight"),
		c.VerifiedWeight.LogUpdates(c, log.LevelTrace, "VerifiedWeight"),
		c.LatestCommitment.LogUpdates(c, log.LevelTrace, "LatestCommitment", (*Commitment).LogName),
		c.LatestProducedCommitment.LogUpdates(c, log.LevelDebug, "LatestProducedCommitment", (*Commitment).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.RequestBlocks.LogUpdates(c, log.LevelDebug, "RequestBlocks"),
		c.SpawnedEngine.LogUpdates(c, log.LevelTrace, "SpawnedEngine", (*engine.Engine).LogName),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdownLogger,
	)
}

// initDerivedProperties initializes the behavior of this chain by setting up the relations between its properties.
func (c *Chain) initDerivedProperties(latestSeenSlot reactive.ReadableVariable[iotago.SlotIndex]) (teardown func()) {
	return lo.Batch(
		c.deriveClaimedWeight(),
		c.deriveVerifiedWeight(),
		c.deriveLatestAttestedWeight(),
		c.deriveWarpSyncMode(),

		c.ForkingPoint.WithValue(func(forkingPoint *Commitment) (teardown func()) {
			return c.deriveParent(forkingPoint)
		}),

		c.Parent.WithNonEmptyValue(func(parent *Chain) (teardown func()) {
			return parent.deriveChildren(c)
		}),

		c.SpawnedEngine.WithNonEmptyValue(func(spawnedEngine *engine.Engine) (teardown func()) {
			return lo.Batch(
				c.deriveWarpSyncThreshold(latestSeenSlot, spawnedEngine),
				c.deriveOutOfSyncThreshold(latestSeenSlot, spawnedEngine),
			)
		}),
	)
}

// deriveWarpSyncMode defines how a chain determines whether it is in warp sync mode or not.
func (c *Chain) deriveWarpSyncMode() func() {
	return c.WarpSyncMode.DeriveValueFrom(reactive.NewDerivedVariable3(func(warpSyncMode bool, latestProducedCommitment *Commitment, warpSyncThreshold iotago.SlotIndex, outOfSyncThreshold iotago.SlotIndex) bool {
		// if we have no latest produced commitment, then the engine is not yet initialized and warp sync is disabled
		if latestProducedCommitment == nil {
			return false
		}

		// if warp sync mode is enabled, keep it enabled until we are no longer below the warp sync threshold
		if warpSyncMode {
			return latestProducedCommitment.ID().Slot() < warpSyncThreshold
		}

		// if warp sync mode is disabled, enable it only if we fall below the out of sync threshold
		return latestProducedCommitment.ID().Slot() < outOfSyncThreshold
	}, c.LatestProducedCommitment, c.WarpSyncThreshold, c.OutOfSyncThreshold, c.WarpSyncMode.Get()))
}

// the AttestedWeight is defined slightly different from the ClaimedWeight and VerifiedWeight, because it is not
// derived from a static value in the commitment but a dynamic value that is derived from the received
// attestations (which may change over time).
func (c *Chain) deriveLatestAttestedWeight() func() {
	return c.LatestAttestedCommitment.WithNonEmptyValue(func(latestAttestedCommitment *Commitment) (teardown func()) {
		return c.AttestedWeight.InheritFrom(latestAttestedCommitment.CumulativeAttestedWeight)
	})
}

func (c *Chain) deriveVerifiedWeight() func() {
	return c.VerifiedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestProducedCommitment *Commitment) uint64 {
		if latestProducedCommitment == nil {
			return 0
		}

		return latestProducedCommitment.CumulativeWeight()
	}, c.LatestProducedCommitment))
}

func (c *Chain) deriveClaimedWeight() (teardown func()) {
	return c.ClaimedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestCommitment *Commitment) uint64 {
		if latestCommitment == nil {
			return 0
		}

		return latestCommitment.CumulativeWeight()
	}, c.LatestCommitment))
}

func (c *Chain) deriveChildren(child *Chain) func() {
	c.Children.Add(child)

	return func() {
		c.Children.Delete(child)
	}
}

func (c *Chain) deriveParent(forkingPoint *Commitment) func() {
	if forkingPoint == nil {
		c.Parent.Set(nil)

		return nil
	}

	return forkingPoint.Parent.WithValue(func(parentCommitment *Commitment) (teardown func()) {
		if parentCommitment == nil {
			c.Parent.Set(nil)

			return nil
		}

		return c.Parent.InheritFrom(parentCommitment.Chain)
	})
}

func (c *Chain) deriveOutOfSyncThreshold(latestSeenSlot reactive.ReadableVariable[iotago.SlotIndex], spawnedEngine *engine.Engine) func() {
	return c.OutOfSyncThreshold.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestSeenSlot iotago.SlotIndex) iotago.SlotIndex {
		outOfSyncOffset := 2 * spawnedEngine.LatestAPI().ProtocolParameters().MaxCommittableAge()
		if outOfSyncOffset >= latestSeenSlot {
			return 0
		}

		return latestSeenSlot - outOfSyncOffset
	}, latestSeenSlot))
}

func (c *Chain) deriveWarpSyncThreshold(latestSeenSlot reactive.ReadableVariable[iotago.SlotIndex], spawnedEngine *engine.Engine) func() {
	return c.WarpSyncThreshold.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestSeenSlot iotago.SlotIndex) iotago.SlotIndex {
		warpSyncOffset := spawnedEngine.LatestAPI().ProtocolParameters().MaxCommittableAge()
		if warpSyncOffset >= latestSeenSlot {
			return 0
		}

		return latestSeenSlot - warpSyncOffset
	}, latestSeenSlot))
}

// registerCommitment registers the given commitment with this chain.
func (c *Chain) registerCommitment(newCommitment *Commitment) (unregister func()) {
	// if a commitment for this slot already exists, then this is a newly forked commitment, that only got associated
	// with this chain because it temporarily inherited it through its parent before forking (ignore it).
	if c.commitments.Compute(newCommitment.Slot(), func(currentCommitment *Commitment, exists bool) *Commitment {
		return lo.Cond(exists, currentCommitment, newCommitment)
	}) != newCommitment {
		return func() {}
	}

	// maxCommitment returns the Commitment object with the higher slot.
	maxCommitment := func(other *Commitment) *Commitment {
		if newCommitment == nil || other != nil && other.Slot() >= newCommitment.Slot() {
			return other
		}

		return newCommitment
	}

	c.LatestCommitment.Compute(maxCommitment)

	unsubscribe := lo.Batch(
		newCommitment.IsAttested.OnTrigger(func() { c.LatestAttestedCommitment.Compute(maxCommitment) }),
		newCommitment.IsVerified.OnTrigger(func() { c.LatestProducedCommitment.Compute(maxCommitment) }),
	)

	return func() {
		unsubscribe()

		c.commitments.Delete(newCommitment.Slot())

		resetToParent := func(latestCommitment *Commitment) *Commitment {
			if latestCommitment == nil || newCommitment.Slot() < latestCommitment.Slot() {
				return latestCommitment
			}

			return newCommitment.Parent.Get()
		}

		c.LatestCommitment.Compute(resetToParent)
		c.LatestAttestedCommitment.Compute(resetToParent)
		c.LatestProducedCommitment.Compute(resetToParent)
	}
}

// claimedWeightVariable is a getter for the ClaimedWeight variable of this chain, which is internally used to be able
// to "address" the variable across multiple chains in a generic way.
func (c *Chain) claimedWeightVariable() reactive.Variable[uint64] {
	return c.ClaimedWeight
}

func (c *Chain) verifiedWeightVariable() reactive.Variable[uint64] {
	return c.VerifiedWeight
}

func (c *Chain) attestedWeightVariable() reactive.Variable[uint64] {
	return c.AttestedWeight
}
