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

	// ParentChain contains the chain that this chain forked from.
	ParentChain reactive.Variable[*Chain]

	// ChildChains contains the set of all chains that forked from this chain.
	ChildChains reactive.Set[*Chain]

	// LatestCommitment contains the latest commitment of this chain.
	LatestCommitment reactive.Variable[*Commitment]

	// LatestAttestedCommitment contains the latest commitment of this chain for which attestations were received.
	LatestAttestedCommitment reactive.Variable[*Commitment]

	// LatestProducedCommitment contains the latest commitment of this chain that we produced ourselves by booking the
	// corresponding blocks in the Engine.
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

	// LatestFullyBookedSlot contains the latest commitment of this chain for which all blocks were booked.
	LatestFullyBookedSlot reactive.Variable[iotago.SlotIndex]

	// OutOfSyncThreshold contains the slot at which the chain will consider itself to be out of sync and switch to warp
	// sync mode. It is derived from the latest network slot minus two times the max committable age.
	OutOfSyncThreshold reactive.Variable[iotago.SlotIndex]

	// RequestAttestations contains a flag that indicates whether this chain should verify the claimed weight by
	// requesting attestations.
	RequestAttestations reactive.Variable[bool]

	// StartEngine contains a flag that indicates whether this chain should verify the state by processing blocks in an
	// engine.
	StartEngine reactive.Variable[bool]

	// Engine contains the engine instance that is used to process blocks for this chain.
	Engine reactive.Variable[*engine.Engine]

	// IsEvicted contains a flag that indicates whether this chain was evicted.
	IsEvicted reactive.Event

	// chains contains a reference to the Chains instance that this chain belongs to.
	chains *Chains

	// commitments contains the commitments that make up this chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// newChain creates a new chain instance.
func newChain(chains *Chains) *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		ParentChain:              reactive.NewVariable[*Chain](),
		ChildChains:              reactive.NewSet[*Chain](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestProducedCommitment: reactive.NewVariable[*Commitment](),
		ClaimedWeight:            reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](),
		VerifiedWeight:           reactive.NewVariable[uint64](),
		WarpSyncMode:             reactive.NewVariable[bool]().Init(true),
		LatestFullyBookedSlot:    reactive.NewVariable[iotago.SlotIndex](),
		OutOfSyncThreshold:       reactive.NewVariable[iotago.SlotIndex](),
		RequestAttestations:      reactive.NewVariable[bool](),
		StartEngine:              reactive.NewVariable[bool](),
		Engine:                   reactive.NewVariable[*engine.Engine](),
		IsEvicted:                reactive.NewEvent(),

		chains:      chains,
		commitments: shrinkingmap.New[iotago.SlotIndex, *Commitment](),
	}

	shutdown := lo.Batch(
		c.initLogger(),
		c.initDerivedProperties(),
	)

	c.IsEvicted.OnTrigger(shutdown)

	return c
}

// WithInitializedEngine is a reactive selector that executes the given callback once an Engine for this chain was
// initialized.
func (c *Chain) WithInitializedEngine(callback func(engineInstance *engine.Engine) (shutdown func())) (shutdown func()) {
	return c.Engine.WithNonEmptyValue(func(engineInstance *engine.Engine) (shutdown func()) {
		return engineInstance.Initialized.WithNonEmptyValue(func(_ bool) (shutdown func()) {
			return callback(engineInstance)
		})
	})
}

// LastCommonSlot returns the slot of the last commitment that is common to this chain and its parent chain.
func (c *Chain) LastCommonSlot() iotago.SlotIndex {
	if forkingPoint := c.ForkingPoint.Get(); forkingPoint != nil {
		if isRoot := forkingPoint.IsRoot.Get(); isRoot {
			return forkingPoint.Slot()
		}

		return forkingPoint.Slot() - 1
	}

	panic("chain has no forking point")
}

// DispatchBlock dispatches the given block to the chain and its children (it is allowed to call this method on a nil
// receiver, in which case it will be a no-op with a return value of false).
func (c *Chain) DispatchBlock(block *model.Block, src peer.ID) (dispatched bool) {
	if c == nil {
		return false
	}

	dispatched = c.dispatchBlockToSpawnedEngine(block, src)

	for _, childChain := range c.ChildChains.ToSlice() {
		dispatched = childChain.DispatchBlock(block, src) || dispatched
	}

	return dispatched
}

// Commitment returns the Commitment for the given slot from the perspective of this chain.
func (c *Chain) Commitment(slot iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c; currentChain != nil; {
		switch forkingPoint := currentChain.ForkingPoint.Get(); {
		case forkingPoint == nil:
			return nil, false
		case forkingPoint.Slot() == slot:
			return forkingPoint, true
		case slot > forkingPoint.Slot():
			return currentChain.commitments.Get(slot)
		default:
			currentChain = c.ParentChain.Get()
		}
	}

	return nil, false
}

// LatestEngine returns the latest engine instance that was spawned by the chain itself or one of its ancestors.
func (c *Chain) LatestEngine() *engine.Engine {
	currentChain, currentEngine := c, c.Engine.Get()
	for ; currentEngine == nil; currentEngine = currentChain.Engine.Get() {
		if currentChain = c.ParentChain.Get(); currentChain == nil {
			return nil
		}
	}

	return currentEngine
}

// initLogger initializes the Logger of this chain.
func (c *Chain) initLogger() (shutdown func()) {
	c.Logger, shutdown = c.chains.NewEntityLogger("")

	return lo.Batch(
		c.WarpSyncMode.LogUpdates(c, log.LevelTrace, "WarpSyncMode"),
		c.LatestFullyBookedSlot.LogUpdates(c, log.LevelTrace, "LatestFullyBookedSlot"),
		c.OutOfSyncThreshold.LogUpdates(c, log.LevelTrace, "OutOfSyncThreshold"),
		c.ForkingPoint.LogUpdates(c, log.LevelTrace, "ForkingPoint", (*Commitment).LogName),
		c.ClaimedWeight.LogUpdates(c, log.LevelTrace, "ClaimedWeight"),
		c.AttestedWeight.LogUpdates(c, log.LevelTrace, "AttestedWeight"),
		c.VerifiedWeight.LogUpdates(c, log.LevelTrace, "VerifiedWeight"),
		c.LatestCommitment.LogUpdates(c, log.LevelTrace, "LatestCommitment", (*Commitment).LogName),
		c.LatestAttestedCommitment.LogUpdates(c, log.LevelTrace, "LatestAttestedCommitment", (*Commitment).LogName),
		c.LatestProducedCommitment.LogUpdates(c, log.LevelDebug, "LatestProducedCommitment", (*Commitment).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.StartEngine.LogUpdates(c, log.LevelDebug, "StartEngine"),
		c.Engine.LogUpdates(c, log.LevelTrace, "Engine", (*engine.Engine).LogName),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdown,
	)
}

// initDerivedProperties initializes the behavior of this chain by setting up the relations between its properties.
func (c *Chain) initDerivedProperties() (shutdown func()) {
	return lo.Batch(
		c.deriveClaimedWeight(),
		c.deriveVerifiedWeight(),
		c.deriveLatestAttestedWeight(),
		c.deriveWarpSyncMode(),

		c.ForkingPoint.WithValue(func(forkingPoint *Commitment) (shutdown func()) {
			return c.deriveParentChain(forkingPoint)
		}),

		c.ParentChain.WithNonEmptyValue(func(parentChain *Chain) (shutdown func()) {
			return parentChain.deriveChildChains(c)
		}),

		c.Engine.WithNonEmptyValue(func(engineInstance *engine.Engine) (shutdown func()) {
			return c.deriveOutOfSyncThreshold(engineInstance)
		}),
	)
}

// deriveWarpSyncMode defines how a chain determines whether it is in warp sync mode or not.
func (c *Chain) deriveWarpSyncMode() func() {
	return c.WarpSyncMode.DeriveValueFrom(reactive.NewDerivedVariable3(func(warpSyncMode bool, latestFullyBookedSlot iotago.SlotIndex, latestSeenSlot iotago.SlotIndex, outOfSyncThreshold iotago.SlotIndex) bool {
		// if warp sync mode is enabled, keep it enabled until we have fully booked all slots
		if warpSyncMode {
			return latestFullyBookedSlot < latestSeenSlot
		}

		// if warp sync mode is disabled, enable it only if we fall below the out of sync threshold
		return latestFullyBookedSlot < outOfSyncThreshold
	}, c.LatestFullyBookedSlot, c.chains.LatestSeenSlot, c.OutOfSyncThreshold, c.WarpSyncMode.Get()))
}

// deriveClaimedWeight defines how a chain determines its claimed weight (by setting the cumulative weight of the
// latest commitment).
func (c *Chain) deriveClaimedWeight() (shutdown func()) {
	return c.ClaimedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestCommitment *Commitment) uint64 {
		if latestCommitment == nil {
			return 0
		}

		return latestCommitment.CumulativeWeight()
	}, c.LatestCommitment))
}

// deriveLatestAttestedWeight defines how a chain determines its attested weight (by inheriting the cumulative attested
// weight of the latest attested commitment). It uses inheritance instead of simply setting the value as the cumulative
// attested weight can change over time depending on the attestations that are received.
func (c *Chain) deriveLatestAttestedWeight() func() {
	return c.LatestAttestedCommitment.WithNonEmptyValue(func(latestAttestedCommitment *Commitment) (shutdown func()) {
		return c.AttestedWeight.InheritFrom(latestAttestedCommitment.CumulativeAttestedWeight)
	})
}

// deriveVerifiedWeight defines how a chain determines its verified weight (by setting the cumulative weight of the
// latest produced commitment).
func (c *Chain) deriveVerifiedWeight() func() {
	return c.VerifiedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestProducedCommitment *Commitment) uint64 {
		if latestProducedCommitment == nil {
			return 0
		}

		return latestProducedCommitment.CumulativeWeight()
	}, c.LatestProducedCommitment))
}

// deriveChildChains defines how a chain determines its ChildChains (by adding each child to the set).
func (c *Chain) deriveChildChains(child *Chain) func() {
	c.ChildChains.Add(child)

	return func() {
		c.ChildChains.Delete(child)
	}
}

// deriveParentChain defines how a chain determines its parent chain from its forking point (it inherits the Chain from
// the parent commitment of the forking point or nil if either of them is still unknown).
func (c *Chain) deriveParentChain(forkingPoint *Commitment) (shutdown func()) {
	if forkingPoint != nil {
		return forkingPoint.Parent.WithValue(func(parentCommitment *Commitment) (shutdown func()) {
			if parentCommitment != nil {
				return c.ParentChain.InheritFrom(parentCommitment.Chain)
			}

			c.ParentChain.Set(nil)

			return nil
		})
	}

	c.ParentChain.Set(nil)

	return nil
}

// deriveOutOfSyncThreshold defines how a chain determines its "out of sync" threshold (the latest seen slot minus 2
// times the max committable age or 0 if this would cause an overflow to the negative numbers).
func (c *Chain) deriveOutOfSyncThreshold(engineInstance *engine.Engine) func() {
	return c.OutOfSyncThreshold.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestSeenSlot iotago.SlotIndex) iotago.SlotIndex {
		return outOfSyncThreshold(engineInstance, latestSeenSlot)
	}, c.chains.LatestSeenSlot))
}

// addCommitment adds the given commitment to this chain.
func (c *Chain) addCommitment(newCommitment *Commitment) (shutdown func()) {
	c.commitments.Set(newCommitment.Slot(), newCommitment)

	c.LatestCommitment.Set(newCommitment)

	return lo.Batch(
		newCommitment.IsAttested.OnTrigger(func() { c.LatestAttestedCommitment.Set(newCommitment) }),
		newCommitment.IsCommitted.OnTrigger(func() { c.LatestProducedCommitment.Set(newCommitment) }),
		newCommitment.IsFullyBooked.OnTrigger(func() { c.LatestFullyBookedSlot.Set(newCommitment.Slot()) }),
	)
}

// dispatchBlockToSpawnedEngine dispatches the given block to the spawned engine of this chain (if it exists).
func (c *Chain) dispatchBlockToSpawnedEngine(block *model.Block, src peer.ID) (dispatched bool) {
	// abort if we do not have a spawned engine
	engineInstance := c.Engine.Get()
	if engineInstance == nil {
		return false
	}

	// abort if the target slot is below the latest commitment
	issuingTime := block.ProtocolBlock().Header.IssuingTime
	targetSlot := engineInstance.APIForTime(issuingTime).TimeProvider().SlotFromTime(issuingTime)
	if targetSlot <= engineInstance.LatestCommitment.Get().Slot() {
		return false
	}

	// perform additional checks if we are in warp sync mode (only let blocks pass that we requested)
	if c.WarpSyncMode.Get() {
		// abort if the target commitment does not exist
		targetCommitment, targetCommitmentExists := c.Commitment(targetSlot)
		if !targetCommitmentExists {
			return false
		}

		// abort if the block is not part of the blocks to warp sync
		blocksToWarpSync := targetCommitment.BlocksToWarpSync.Get()
		if blocksToWarpSync == nil || !blocksToWarpSync.Has(block.ID()) {
			return false
		}
	}

	// dispatch the block to the spawned engine if all previous checks passed
	engineInstance.ProcessBlockFromPeer(block, src)

	return true
}

// claimedWeight is a getter for the ClaimedWeight variable of this chain, which is internally used to be able to
// "address" the variable across multiple chains in a generic way.
func (c *Chain) claimedWeight() reactive.Variable[uint64] {
	return c.ClaimedWeight
}

// verifiedWeight is a getter for the VerifiedWeight variable of this chain, which is internally used to be able to
// "address" the variable across multiple chains in a generic way.
func (c *Chain) verifiedWeight() reactive.Variable[uint64] {
	return c.VerifiedWeight
}

// attestedWeight is a getter for the AttestedWeight variable of this chain, which is internally used to be able to
// "address" the variable across multiple chains in a generic way.
func (c *Chain) attestedWeight() reactive.Variable[uint64] {
	return c.AttestedWeight
}

// outOfSyncThreshold returns the slot index at which the node is considered out of sync.
func outOfSyncThreshold(engineInstance *engine.Engine, latestSeenSlot iotago.SlotIndex) iotago.SlotIndex {
	if outOfSyncOffset := 2 * engineInstance.LatestAPI().ProtocolParameters().MaxCommittableAge(); outOfSyncOffset < latestSeenSlot {
		return latestSeenSlot - outOfSyncOffset
	}

	return 0
}
