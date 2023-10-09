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
	SyncThreshold            reactive.Variable[iotago.SlotIndex]
	WarpSyncThreshold        reactive.Variable[iotago.SlotIndex]
	VerifyAttestations       reactive.Variable[bool]
	VerifyState              reactive.Variable[bool]
	Engine                   reactive.Variable[*engine.Engine]
	IsSolid                  reactive.Event
	IsEvicted                reactive.Event

	commitments   *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]
	parentEngine  reactive.Variable[*engine.Engine]
	SpawnedEngine reactive.Variable[*engine.Engine]

	log.Logger
}

func NewChain(logger log.Logger) *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		ParentChain:              reactive.NewVariable[*Chain](),
		ChildChains:              reactive.NewSet[*Chain](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		VerifyAttestations:       reactive.NewVariable[bool](),
		IsEvicted:                reactive.NewEvent(),

		commitments:   shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		parentEngine:  reactive.NewVariable[*engine.Engine](),
		VerifyState:   reactive.NewVariable[bool](),
		SpawnedEngine: reactive.NewVariable[*engine.Engine](),
	}

	c.ClaimedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestCommitment)
	c.AttestedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeAttestedWeight, c.LatestAttestedCommitment)
	c.VerifiedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestVerifiedCommitment)

	c.WarpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitment *Commitment) iotago.SlotIndex {
		if latestCommitment == nil || latestCommitment.Slot() < WarpSyncOffset {
			return 0
		}

		return latestCommitment.Slot() - WarpSyncOffset
	}, c.LatestCommitment)

	c.SyncThreshold = reactive.NewDerivedVariable2[iotago.SlotIndex](func(forkingPoint, latestVerifiedCommitment *Commitment) iotago.SlotIndex {
		if forkingPoint == nil {
			return SyncWindow + 1
		}

		if latestVerifiedCommitment == nil {
			return forkingPoint.Slot() + SyncWindow + 1
		}

		return latestVerifiedCommitment.Slot() + SyncWindow + 1
	}, c.ForkingPoint, c.LatestVerifiedCommitment)

	c.Engine = reactive.NewDerivedVariable2(func(spawnedEngine, parentEngine *engine.Engine) *engine.Engine {
		if spawnedEngine != nil {
			return spawnedEngine
		}

		return parentEngine
	}, c.SpawnedEngine, c.parentEngine)

	c.ParentChain.OnUpdate(func(prevParent, newParent *Chain) {
		if prevParent != nil {
			prevParent.ChildChains.Delete(c)
		}

		if newParent != nil {
			newParent.ChildChains.Add(c)
		}
	})

	c.ForkingPoint.OnUpdateWithContext(func(_, forkingPoint *Commitment, forkingPointContext func(subscriptionFactory func() (unsubscribe func()))) {
		forkingPointContext(func() func() {
			return forkingPoint.Parent.OnUpdate(func(_, parent *Commitment) {
				forkingPointContext(func() func() {
					return lo.Batch(
						c.ParentChain.InheritFrom(parent.Chain),
						c.parentEngine.InheritFrom(parent.Engine),
					)
				})
			})
		})
	})

	c.Logger = logger.NewEntityLogger("Chain", c.IsEvicted, func(entityLogger log.Logger) {
		c.ForkingPoint.LogUpdates(entityLogger, log.LevelTrace, "ForkingPoint", (*Commitment).LogName)
		c.ClaimedWeight.LogUpdates(entityLogger, log.LevelTrace, "ClaimedWeight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.VerifiedWeight.LogUpdates(entityLogger, log.LevelTrace, "VerifiedWeight")
		c.LatestCommitment.LogUpdates(entityLogger, log.LevelTrace, "LatestCommitment", (*Commitment).LogName)
		c.LatestVerifiedCommitment.LogUpdates(entityLogger, log.LevelDebug, "LatestVerifiedCommitment", (*Commitment).LogName)
		c.VerifyAttestations.LogUpdates(entityLogger, log.LevelTrace, "VerifyAttestations")
		c.VerifyState.LogUpdates(entityLogger, log.LevelDebug, "VerifyState")
	})

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

	success = true
	for _, chain := range append([]*Chain{c}, c.ChildChains.ToSlice()...) {
		if targetSlot := block.ID().Slot(); chain.VerifyState.Get() && chain.earliestUncommittedSlot() <= targetSlot {
			if targetEngine := chain.SpawnedEngine.Get(); targetEngine != nil && targetSlot < c.SyncThreshold.Get() {
				targetEngine.ProcessBlockFromPeer(block, src)
			} else {
				success = false
			}
		}
	}

	return success
}

func (c *Chain) earliestUncommittedSlot() iotago.SlotIndex {
	if latestVerifiedCommitment := c.LatestVerifiedCommitment.Get(); latestVerifiedCommitment != nil {
		return latestVerifiedCommitment.Slot() + 1
	} else if forkingPoint := c.ForkingPoint.Get(); forkingPoint != nil {
		return forkingPoint.Slot()
	} else {
		return 0
	}
}

func (c *Chain) InSyncRange(slot iotago.SlotIndex) bool {
	if latestVerifiedCommitment := c.LatestVerifiedCommitment.Get(); latestVerifiedCommitment != nil {
		return slot > latestVerifiedCommitment.Slot() && slot < c.SyncThreshold.Get()
	}

	forkingPoint := c.ForkingPoint.Get()

	return forkingPoint != nil && (slot > forkingPoint.Slot()-1 && slot < c.SyncThreshold.Get())
}

func (c *Chain) registerCommitment(commitment *Commitment) (unregister func()) {
	c.commitments.Set(commitment.Slot(), commitment)

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
			if latestCommitment == nil || commitment.Slot() > latestCommitment.Slot() {
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
