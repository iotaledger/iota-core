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

type Chain struct {
	ForkingPoint             reactive.Variable[*Commitment]
	Parent                   reactive.Variable[*Chain]
	Children                 reactive.Set[*Chain]
	LatestCommitment         reactive.Variable[*Commitment]
	LatestAttestedCommitment reactive.Variable[*Commitment]
	LatestVerifiedCommitment reactive.Variable[*Commitment]
	ClaimedWeight            reactive.Variable[uint64]
	AttestedWeight           reactive.Variable[uint64]
	VerifiedWeight           reactive.Variable[uint64]
	LatestNetworkSlot        reactive.Variable[iotago.SlotIndex]
	WarpSyncMode             reactive.Variable[bool]
	WarpSyncThreshold        reactive.Variable[iotago.SlotIndex]
	OutOfSyncThreshold       reactive.Variable[iotago.SlotIndex]
	VerifyAttestations       reactive.Variable[bool]
	VerifyState              reactive.Variable[bool]
	SpawnedEngine            reactive.Variable[*engine.Engine]
	Engine                   reactive.Variable[*engine.Engine]
	IsSolid                  reactive.Event
	IsEvicted                reactive.Event

	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	log.Logger
}

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
		VerifyAttestations:       reactive.NewVariable[bool](),
		VerifyState:              reactive.NewVariable[bool](),
		SpawnedEngine:            reactive.NewVariable[*engine.Engine](),
		Engine:                   reactive.NewVariable[*engine.Engine](),
		IsEvicted:                reactive.NewEvent(),

		commitments: shrinkingmap.New[iotago.SlotIndex, *Commitment](),
	}).initLogging(chains).initBehavior(chains)
}

func (c *Chain) initLogging(chains *Chains) (self *Chain) {
	var shutdownLogger func()
	c.Logger, shutdownLogger = chains.protocol.NewEntityLogger("Chain")

	teardownLogging := lo.Batch(
		c.Engine.LogUpdates(c, log.LevelTrace, "Engine", (*engine.Engine).LogName),
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
		c.VerifyAttestations.LogUpdates(c, log.LevelTrace, "VerifyAttestations"),
		c.VerifyState.LogUpdates(c, log.LevelDebug, "VerifyState"),
		c.SpawnedEngine.LogUpdates(c, log.LevelTrace, "SpawnedEngine", (*engine.Engine).LogName),
		c.Engine.LogUpdates(c, log.LevelTrace, "SpawnedEngine", (*engine.Engine).LogName),
		c.IsEvicted.LogUpdates(c, log.LevelTrace, "IsEvicted"),

		shutdownLogger,
	)

	c.IsEvicted.OnTrigger(teardownLogging)

	return c
}

func (c *Chain) initBehavior(chains *Chains) (self *Chain) {
	teardownBehavior := lo.Batch(
		c.Parent.WithValue(func(parent *Chain) (teardown func()) {
			if parent == nil {
				return c.Engine.InheritFrom(c.SpawnedEngine)
			}

			return lo.Batch(
				parent.registerChild(c),

				c.Engine.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ *engine.Engine, spawnedEngine *engine.Engine, parentEngine *engine.Engine) *engine.Engine {
					if spawnedEngine != nil {
						return spawnedEngine
					}

					return parentEngine
				}, c.SpawnedEngine, parent.Engine)),
			)
		}),

		c.SpawnedEngine.WithNonEmptyValue(func(engine *engine.Engine) (teardown func()) {
			return lo.Batch(
				c.LatestNetworkSlot.DeriveValueFrom(reactive.NewDerivedVariable(func(latestNetworkSlot iotago.SlotIndex, latestNetworkTime time.Time) iotago.SlotIndex {
					return engine.LatestAPI().TimeProvider().SlotFromTime(latestNetworkTime)
				}, chains.protocol.NetworkClock)),

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

func (c *Chain) Commitment(slot iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c; currentChain != nil; {
		switch forkingPoint := currentChain.ForkingPoint.Get(); {
		case forkingPoint == nil:
			return nil, false // this should never happen, but we can handle it gracefully anyway
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
	if c != nil {
		for _, chain := range append([]*Chain{c}, c.Children.ToSlice()...) {
			targetEngine := chain.SpawnedEngine.Get()
			if targetEngine == nil {
				continue
			}

			if chain.WarpSyncMode.Get() {
				issuingTime := block.ProtocolBlock().Header.IssuingTime

				targetCommitment, targetCommitmentExists := chain.Commitment(targetEngine.APIForTime(issuingTime).TimeProvider().SlotFromTime(issuingTime))
				if !targetCommitmentExists {
					continue
				}

				if blocksToWarpSync := targetCommitment.BlocksToWarpSync.Get(); blocksToWarpSync == nil || !blocksToWarpSync.Has(block.ID()) {
					continue
				}
			}

			targetEngine.ProcessBlockFromPeer(block, src)

			success = true
		}
	}

	return success
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

func (c *Chain) registerChild(child *Chain) (unregisterChild func()) {
	c.Children.Add(child)

	return func() {
		c.Children.Delete(child)
	}
}
