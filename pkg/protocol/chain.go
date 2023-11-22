package protocol

import (
	"time"

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

	// LatestVerifiedCommitment contains the latest commitment of this chain that we produced by an engine instance.
	LatestVerifiedCommitment reactive.Variable[*Commitment]

	// ClaimedWeight contains the claimed weight of this chain which is derived from the cumulative weight of the
	// LatestCommitment.
	ClaimedWeight reactive.Variable[uint64]

	// AttestedWeight contains the attested weight of this chain which is derived from the cumulative weight of all
	// attestations up to the LatestAttestedCommitment.
	AttestedWeight reactive.Variable[uint64]

	// VerifiedWeight contains the verified weight of this chain which is derived from the cumulative weight of the
	// latest verified commitment.
	VerifiedWeight reactive.Variable[uint64]

	// LatestNetworkSlot contains the latest network slot.
	LatestNetworkSlot reactive.Variable[iotago.SlotIndex]

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

	// RequestBlocks contains a flag that indicates whether this chain should verify the state by requesting blocks.
	RequestBlocks reactive.Variable[bool]

	// SpawnedEngine contains the engine that was spawned by this chain.
	SpawnedEngine reactive.Variable[*engine.Engine]

	// IsEvicted contains a flag that indicates whether this chain was evicted.
	IsEvicted reactive.Event

	// commitments contains the commitments that make up this chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	// Logger embeds a logger that can be used to log messages emitted by this chain.
	log.Logger
}

// NewChain creates a new chain instance.
func NewChain(chains *Chains) *Chain {
	return (&Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		Parent:                   reactive.NewVariable[*Chain](),
		Children:                 reactive.NewSet[*Chain](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		ClaimedWeight:            reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](),
		VerifiedWeight:           reactive.NewVariable[uint64](),
		WarpSyncMode:             reactive.NewVariable[bool]().Init(true),
		LatestNetworkSlot:        reactive.NewVariable[iotago.SlotIndex](),
		WarpSyncThreshold:        reactive.NewVariable[iotago.SlotIndex](),
		OutOfSyncThreshold:       reactive.NewVariable[iotago.SlotIndex](),
		RequestAttestations:      reactive.NewVariable[bool](),
		RequestBlocks:            reactive.NewVariable[bool](),
		SpawnedEngine:            reactive.NewVariable[*engine.Engine](),
		IsEvicted:                reactive.NewEvent(),

		commitments: shrinkingmap.New[iotago.SlotIndex, *Commitment](),
	}).initLogging(chains).initBehavior(chains)
}

// Engine returns the engine that is spawned by this chain.
func (c *Chain) Engine() *engine.Engine {
	currentChain, currentEngine := c, c.SpawnedEngine.Get()
	for currentEngine == nil {
		if currentChain = c.Parent.Get(); currentChain == nil {
			return nil
		}

		currentEngine = currentChain.SpawnedEngine.Get()
	}

	return currentEngine
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

func (c *Chain) DispatchBlock(block *model.Block, src peer.ID) (success bool) {
	if c == nil {
		return false
	}

	for _, chain := range append([]*Chain{c}, c.Children.ToSlice()...) {
		spawnedEngine := chain.SpawnedEngine.Get()
		if spawnedEngine == nil {
			continue
		}

		if chain.WarpSyncMode.Get() {
			issuingTime := block.ProtocolBlock().Header.IssuingTime

			targetCommitment, targetCommitmentExists := chain.Commitment(spawnedEngine.APIForTime(issuingTime).TimeProvider().SlotFromTime(issuingTime))
			if !targetCommitmentExists {
				continue
			}

			if blocksToWarpSync := targetCommitment.BlocksToWarpSync.Get(); blocksToWarpSync == nil || !blocksToWarpSync.Has(block.ID()) {
				continue
			}
		}

		spawnedEngine.ProcessBlockFromPeer(block, src)

		success = true
	}

	return success
}

// initLogging initializes the logging of changes to the properties of this chain.
func (c *Chain) initLogging(chains *Chains) (self *Chain) {
	var shutdownLogger func()
	c.Logger, shutdownLogger = chains.protocol.NewEntityLogger("Chain")

	teardownLogging := lo.Batch(
		c.WarpSyncMode.LogUpdates(c, log.LevelTrace, "WarpSyncMode"),
		c.LatestNetworkSlot.LogUpdates(c, log.LevelTrace, "LatestNetworkSlot"),
		c.WarpSyncThreshold.LogUpdates(c, log.LevelTrace, "WarpSyncThreshold"),
		c.OutOfSyncThreshold.LogUpdates(c, log.LevelTrace, "OutOfSyncThreshold"),
		c.ForkingPoint.LogUpdates(c, log.LevelTrace, "ForkingPoint", (*Commitment).LogName),
		c.ClaimedWeight.LogUpdates(c, log.LevelTrace, "ClaimedWeight"),
		c.AttestedWeight.LogUpdates(c, log.LevelTrace, "AttestedWeight"),
		c.VerifiedWeight.LogUpdates(c, log.LevelTrace, "VerifiedWeight"),
		c.LatestCommitment.LogUpdates(c, log.LevelTrace, "LatestCommitment", (*Commitment).LogName),
		c.LatestVerifiedCommitment.LogUpdates(c, log.LevelDebug, "LatestVerifiedCommitment", (*Commitment).LogName),
		c.RequestAttestations.LogUpdates(c, log.LevelTrace, "RequestAttestations"),
		c.RequestBlocks.LogUpdates(c, log.LevelDebug, "RequestBlocks"),
		c.SpawnedEngine.LogUpdates(c, log.LevelTrace, "SpawnedEngine", (*engine.Engine).LogName),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdownLogger,
	)

	c.IsEvicted.OnTrigger(teardownLogging)

	return c
}

// initBehavior initializes the behavior of this chain by setting up the relations between its properties.
func (c *Chain) initBehavior(chains *Chains) (self *Chain) {
	teardownBehavior := lo.Batch(
		c.Parent.WithNonEmptyValue(func(parent *Chain) (teardown func()) {
			parent.Children.Add(c)

			return func() {
				parent.Children.Delete(c)
			}
		}),

		c.SpawnedEngine.WithNonEmptyValue(func(engine *engine.Engine) (teardown func()) {
			return lo.Batch(
				c.LatestNetworkSlot.DeriveValueFrom(reactive.NewDerivedVariable(func(latestNetworkSlot iotago.SlotIndex, latestNetworkTime time.Time) iotago.SlotIndex {
					return engine.LatestAPI().TimeProvider().SlotFromTime(latestNetworkTime)
				}, chains.protocol.Clock)),

				c.WarpSyncThreshold.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestNetworkSlot iotago.SlotIndex) iotago.SlotIndex {
					warpSyncOffset := engine.LatestAPI().ProtocolParameters().MaxCommittableAge()
					if warpSyncOffset >= latestNetworkSlot {
						return 0
					}

					return latestNetworkSlot - warpSyncOffset
				}, c.LatestNetworkSlot)),

				c.OutOfSyncThreshold.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestNetworkSlot iotago.SlotIndex) iotago.SlotIndex {
					outOfSyncOffset := 2 * engine.LatestAPI().ProtocolParameters().MaxCommittableAge()
					if outOfSyncOffset >= latestNetworkSlot {
						return 0
					}

					return latestNetworkSlot - outOfSyncOffset
				}, c.LatestNetworkSlot)),
			)
		}),

		c.ForkingPoint.WithValue(func(forkingPoint *Commitment) (teardown func()) {
			if forkingPoint == nil {
				c.Parent.Set(nil)

				return func() {}
			}

			return forkingPoint.Parent.WithValue(func(parentCommitment *Commitment) (teardown func()) {
				if parentCommitment == nil {
					c.Parent.Set(nil)

					return func() {}
				}

				return c.Parent.InheritFrom(parentCommitment.Chain)
			})
		}),

		c.ClaimedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestCommitment *Commitment) uint64 {
			if latestCommitment == nil {
				return 0
			}

			return latestCommitment.CumulativeWeight()
		}, c.LatestCommitment)),

		c.VerifiedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, latestVerifiedCommitment *Commitment) uint64 {
			if latestVerifiedCommitment == nil {
				return 0
			}

			return latestVerifiedCommitment.CumulativeWeight()
		}, c.LatestVerifiedCommitment)),

		// the AttestedWeight is defined slightly different from the ClaimedWeight and VerifiedWeight, because it is not
		// derived from a static value in the commitment but a dynamic value that is derived from the received
		// attestations (which may change over time).
		c.LatestAttestedCommitment.WithNonEmptyValue(func(latestAttestedCommitment *Commitment) (teardown func()) {
			return c.AttestedWeight.InheritFrom(latestAttestedCommitment.CumulativeAttestedWeight)
		}),

		c.WarpSyncMode.DeriveValueFrom(reactive.NewDerivedVariable3(func(warpSync bool, latestVerifiedCommitment *Commitment, warpSyncThreshold iotago.SlotIndex, outOfSyncThreshold iotago.SlotIndex) bool {
			return latestVerifiedCommitment != nil && lo.Cond(warpSync, latestVerifiedCommitment.ID().Slot() < warpSyncThreshold, latestVerifiedCommitment.ID().Slot() < outOfSyncThreshold)
		}, c.LatestVerifiedCommitment, c.WarpSyncThreshold, c.OutOfSyncThreshold, c.WarpSyncMode.Get())),
	)

	c.IsEvicted.OnTrigger(teardownBehavior)

	return c
}

func (c *Chain) registerCommitment(newCommitment *Commitment) (unregister func()) {
	// if a commitment for this slot already exists, then this is a newly forked commitment, that only got associated
	// with this chain because it temporarily inherited it through its parent before forking its own chain
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
		newCommitment.IsVerified.OnTrigger(func() { c.LatestVerifiedCommitment.Compute(maxCommitment) }),
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
		c.LatestVerifiedCommitment.Compute(resetToParent)
	}
}
