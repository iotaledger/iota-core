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
	ParentChain              reactive.Variable[*Chain]
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

func NewChain(protocol *Protocol) *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		ParentChain:              reactive.NewVariable[*Chain](),
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
	}

	c.Logger = protocol.NewEntityLogger("Chain", c.IsEvicted, func(chainLogger log.Logger) {
		chainLogger.LogDebug("created")

		c.WarpSync.LogUpdates(chainLogger, log.LevelTrace, "WarpSync")
		c.NetworkClockSlot.LogUpdates(chainLogger, log.LevelTrace, "NetworkClockSlot")
		c.WarpSyncThreshold.LogUpdates(chainLogger, log.LevelTrace, "WarpSyncThreshold")
		c.OutOfSyncThreshold.LogUpdates(chainLogger, log.LevelTrace, "OutOfSyncThreshold")
		c.ForkingPoint.LogUpdates(chainLogger, log.LevelTrace, "ForkingPoint", (*Commitment).LogName)
		c.ClaimedWeight.LogUpdates(chainLogger, log.LevelTrace, "ClaimedWeight")
		c.AttestedWeight.LogUpdates(chainLogger, log.LevelTrace, "AttestedWeight")
		c.VerifiedWeight.LogUpdates(chainLogger, log.LevelTrace, "VerifiedWeight")
		c.LatestCommitment.LogUpdates(chainLogger, log.LevelTrace, "LatestCommitment", (*Commitment).LogName)
		c.LatestVerifiedCommitment.LogUpdates(chainLogger, log.LevelDebug, "LatestVerifiedCommitment", (*Commitment).LogName)
		c.VerifyAttestations.LogUpdates(chainLogger, log.LevelTrace, "VerifyAttestations")
		c.VerifyState.LogUpdates(chainLogger, log.LevelDebug, "VerifyState")
		c.SpawnedEngine.LogUpdates(chainLogger, log.LevelDebug, "SpawnedEngine", (*engine.Engine).LogName)
		c.IsEvicted.LogUpdates(chainLogger, log.LevelDebug, "IsEvicted")
	})

	c.initClaimedWeight()
	c.initAttestedWeight()
	c.initVerifiedWeight()
	c.initEngine()
	c.initWarpSync()

	protocol.NetworkClock.OnUpdate(func(_ time.Time, now time.Time) {
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

	c.ParentChain.OnUpdate(func(prevParent *Chain, newParent *Chain) {
		if prevParent != nil {
			prevParent.ChildChains.Delete(c)
		}

		if newParent != nil {
			newParent.ChildChains.Add(c)
		}
	})

	c.ForkingPoint.OnUpdateWithContext(func(_ *Commitment, forkingPoint *Commitment, forkingPointContext func(subscriptionFactory func() (unsubscribe func()))) {
		forkingPointContext(func() func() {
			return forkingPoint.Parent.OnUpdate(func(_ *Commitment, parent *Commitment) {
				forkingPointContext(func() func() {
					return c.ParentChain.InheritFrom(parent.Chain)
				})
			})
		})
	})

	return c
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

func (c *Chain) initEngine() {
	c.ParentChain.OnUpdateWithContext(func(_ *Chain, parentChain *Chain, unsubscribeOnUpdate func(subscriptionFactory func() (unsubscribe func()))) {
		unsubscribeOnUpdate(func() func() {
			if parentChain == nil {
				return c.Engine.InheritFrom(c.SpawnedEngine)
			}

			return c.Engine.DeriveValueFrom(reactive.NewDerivedVariable2(func(_ *engine.Engine, spawnedEngine *engine.Engine, parentEngine *engine.Engine) *engine.Engine {
				if spawnedEngine != nil {
					return spawnedEngine
				}

				return parentEngine
			}, c.SpawnedEngine, parentChain.Engine))
		})
	}, true)
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
		if chain.VerifyState.Get() {
			if targetEngine := chain.Engine.Get(); targetEngine != nil && !chain.WarpSync.Get() || targetEngine.BlockRequester.HasTicker(block.ID()) {
				targetEngine.ProcessBlockFromPeer(block, src)

				success = true
			}
		}
	}

	return success
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

func (c *Chain) claimedWeight() reactive.Variable[uint64] {
	return c.ClaimedWeight
}

func (c *Chain) attestedWeight() reactive.Variable[uint64] {
	return c.AttestedWeight
}

func (c *Chain) verifiedWeight() reactive.Variable[uint64] {
	return c.VerifiedWeight
}
