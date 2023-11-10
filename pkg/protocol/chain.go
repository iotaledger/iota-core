package protocol

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chain struct {
	ForkingPoint             reactive.Variable[*Commitment]
	ChildChains              reactive.Set[*Chain]
	LatestCommitment         reactive.Variable[*Commitment]
	LatestAttestedCommitment reactive.Variable[*Commitment]
	LatestVerifiedCommitment reactive.Variable[*Commitment]
	ClaimedWeight            reactive.Variable[uint64]
	AttestedWeight           reactive.Variable[uint64]
	VerifiedWeight           reactive.Variable[uint64]
	NetworkClockSlot         reactive.Variable[iotago.SlotIndex]
	WarpSync                 reactive.Variable[bool]
	WarpSyncThreshold        reactive.Variable[iotago.SlotIndex]
	OutOfSyncThreshold       reactive.Variable[iotago.SlotIndex]
	VerifyAttestations       reactive.Variable[bool]
	VerifyState              reactive.Variable[bool]
	Engine                   reactive.Variable[*engine.Engine]
	IsSolid                  reactive.Event
	IsEvicted                reactive.Event

	commitments   *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]
	SpawnedEngine reactive.Variable[*engine.Engine]

	log.Logger
}

func NewChain(chains *Chains) *Chain {
	c := (&Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		ChildChains:              reactive.NewSet[*Chain](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		ClaimedWeight:            reactive.NewVariable[uint64](),
		AttestedWeight:           reactive.NewVariable[uint64](),
		VerifiedWeight:           reactive.NewVariable[uint64](),
		WarpSync:                 reactive.NewVariable[bool]().Init(true),
		NetworkClockSlot:         reactive.NewVariable[iotago.SlotIndex](),
		WarpSyncThreshold:        reactive.NewVariable[iotago.SlotIndex](),
		OutOfSyncThreshold:       reactive.NewVariable[iotago.SlotIndex](),
		VerifyAttestations:       reactive.NewVariable[bool](),
		Engine:                   reactive.NewVariable[*engine.Engine](),
		IsEvicted:                reactive.NewEvent(),

		commitments:   shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		VerifyState:   reactive.NewVariable[bool](),
		SpawnedEngine: reactive.NewVariable[*engine.Engine](),
	}).initLogging(chains)

	c.initClaimedWeight()
	c.initAttestedWeight()
	c.initVerifiedWeight()
	c.initWarpSync()

	chains.protocol.NetworkClock.OnUpdate(func(_ time.Time, now time.Time) {
		if engineInstance := c.Engine.Get(); engineInstance != nil {
			c.NetworkClockSlot.Set(engineInstance.LatestAPI().TimeProvider().SlotFromTime(now))
		}
	})

	c.NetworkClockSlot.OnUpdate(func(_ iotago.SlotIndex, slot iotago.SlotIndex) {
		engineInstance := c.Engine.Get()
		if engineInstance == nil {
			return
		}

		warpSyncThresholdOffset := engineInstance.LatestAPI().ProtocolParameters().MaxCommittableAge()
		outOfSyncThresholdOffset := 2 * warpSyncThresholdOffset

		if warpSyncThresholdOffset > slot {
			c.WarpSyncThreshold.Set(0)
		} else {
			c.WarpSyncThreshold.Set(slot - warpSyncThresholdOffset)
		}

		if outOfSyncThresholdOffset > slot {
			c.OutOfSyncThreshold.Set(0)
		} else {
			c.OutOfSyncThreshold.Set(slot - outOfSyncThresholdOffset)
		}
	})

	c.ForkingPoint.WithValue(func(forkingPoint *Commitment) (teardown func()) {
		if forkingPoint == nil {
			return c.Engine.InheritFrom(c.SpawnedEngine)
		}

		return forkingPoint.Parent.WithValue(func(parent *Commitment) (teardown func()) {
			if parent == nil {
				return c.Engine.InheritFrom(c.SpawnedEngine)
			}

			return parent.Chain.WithValue(func(parentChain *Chain) (teardown func()) {
				if parentChain == nil {
					return c.Engine.InheritFrom(c.SpawnedEngine)
				}

				return lo.Batch(
					parentChain.registerChild(c),

					c.Engine.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ *engine.Engine, spawnedEngine *engine.Engine, parentEngine *engine.Engine) *engine.Engine {
						if spawnedEngine != nil {
							return spawnedEngine
						}

						return parentEngine
					}, c.SpawnedEngine, parentChain.Engine)),
				)
			})
		})
	})

	return c
}

func (c *Chain) initLogging(chains *Chains) (self *Chain) {
	var shutdownLogger func()
	c.Logger, shutdownLogger = chains.protocol.NewEntityLogger("Chain")

	teardownLogging := lo.Batch(
		c.Engine.LogUpdates(c, log.LevelError, "Engine", (*engine.Engine).LogName),
		c.WarpSync.LogUpdates(c, log.LevelTrace, "WarpSync"),
		c.NetworkClockSlot.LogUpdates(c, log.LevelTrace, "NetworkClockSlot"),
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
			parent := forkingPoint.Parent.Get()
			if parent == nil {
				return nil, false
			}

			currentChain = parent.Chain.Get()
		}
	}

	return nil, false
}

func (c *Chain) DispatchBlock(block *model.Block, src peer.ID) (success bool) {
	if c == nil {
		return false
	}

	for _, chain := range append([]*Chain{c}, c.ChildChains.ToSlice()...) {
		if !chain.VerifyState.Get() {
			continue
		}

		targetEngine := chain.Engine.Get()
		if targetEngine == nil {
			continue
		} else if issuingTime := block.ProtocolBlock().Header.IssuingTime; chain.WarpSync.Get() {
			if targetCommitment, exists := chain.Commitment(targetEngine.APIForTime(issuingTime).TimeProvider().SlotFromTime(issuingTime)); !exists {
				continue
			} else if blocksToWarpSync := targetCommitment.BlocksToWarpSync.Get(); blocksToWarpSync == nil || !blocksToWarpSync.Has(block.ID()) {
				continue
			}
		}

		targetEngine.ProcessBlockFromPeer(block, src)

		success = true
	}

	return success
}

func (c *Chain) initClaimedWeight() {
	c.ClaimedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, c *Commitment) uint64 {
		if c == nil {
			return 0
		}

		return c.CumulativeWeight()
	}, c.LatestCommitment))
}

func (c *Chain) initAttestedWeight() {
	c.LatestAttestedCommitment.OnUpdateWithContext(func(_ *Commitment, latestAttestedCommitment *Commitment, unsubscribeOnUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		if latestAttestedCommitment != nil {
			setupInheritance := func() func() {
				return c.AttestedWeight.InheritFrom(latestAttestedCommitment.CumulativeAttestedWeight)
			}

			unsubscribeOnUpdate(setupInheritance)
		}
	})
}

func (c *Chain) initVerifiedWeight() {
	c.VerifiedWeight.DeriveValueFrom(reactive.NewDerivedVariable(func(_ uint64, c *Commitment) uint64 {
		if c == nil {
			return 0
		}

		return c.CumulativeWeight()
	}, c.LatestVerifiedCommitment))
}

func (c *Chain) initWarpSync() {
	enableWarpSyncIfNecessary := func() (unsubscribe func()) {
		return c.WarpSync.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ bool, latestVerifiedCommitment *Commitment, outOfSyncThreshold iotago.SlotIndex) bool {
			return latestVerifiedCommitment != nil && latestVerifiedCommitment.ID().Slot() < outOfSyncThreshold
		}, c.LatestVerifiedCommitment, c.OutOfSyncThreshold, c.WarpSync.Get()))
	}

	disableWarpSyncIfNecessary := func() (unsubscribe func()) {
		return c.WarpSync.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ bool, latestVerifiedCommitment *Commitment, warpSyncThreshold iotago.SlotIndex) bool {
			return latestVerifiedCommitment != nil && latestVerifiedCommitment.ID().Slot() < warpSyncThreshold
		}, c.LatestVerifiedCommitment, c.WarpSyncThreshold, c.WarpSync.Get()))
	}

	warpSyncTogglePool := workerpool.New("WarpSync toggle", workerpool.WithWorkerCount(1)).Start()
	c.IsEvicted.OnTrigger(func() { warpSyncTogglePool.Shutdown() })

	var unsubscribe func()

	c.WarpSync.OnUpdate(func(_ bool, warpSync bool) {
		if !c.IsEvicted.Get() {
			warpSyncTogglePool.Submit(func() {
				if unsubscribe != nil {
					unsubscribe()
				}

				if warpSync {
					unsubscribe = disableWarpSyncIfNecessary()
				} else {
					unsubscribe = enableWarpSyncIfNecessary()
				}
			})
		}
	})
}

func (c *Chain) registerCommitment(commitment *Commitment) (unregister func()) {
	if c.commitments.Compute(commitment.Slot(), func(currentCommitment *Commitment, exists bool) *Commitment {
		if !exists {
			return commitment
		}

		return currentCommitment
	}) != commitment {
		return func() {}
	}

	// maxCommitment returns the Commitment object with the higher slot.
	maxCommitment := func(other *Commitment) *Commitment {
		if commitment == nil || other != nil && other.Slot() >= commitment.Slot() {
			return other
		}

		return commitment
	}

	c.LatestCommitment.Compute(maxCommitment)

	unsubscribe := lo.Batch(
		commitment.IsAttested.OnTrigger(func() {
			c.LatestAttestedCommitment.Compute(maxCommitment)
		}),
		commitment.IsVerified.OnTrigger(func() { c.LatestVerifiedCommitment.Compute(maxCommitment) }),
	)

	return func() {
		unsubscribe()

		c.commitments.Delete(commitment.Slot())

		resetToParent := func(latestCommitment *Commitment) *Commitment {
			if latestCommitment == nil || commitment.Slot() < latestCommitment.Slot() {
				return latestCommitment
			}

			return commitment.Parent.Get()
		}

		c.LatestCommitment.Compute(resetToParent)
		c.LatestAttestedCommitment.Compute(resetToParent)
		c.LatestVerifiedCommitment.Compute(resetToParent)
	}
}

func (c *Chain) registerChild(child *Chain) (unregisterChild func()) {
	c.ChildChains.Add(child)

	return func() {
		c.ChildChains.Delete(child)
	}
}
