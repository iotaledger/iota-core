package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chain struct {
	ForkingPoint             reactive.Variable[*Commitment]
	LatestCommitment         reactive.Variable[*Commitment]
	LatestAttestedCommitment reactive.Variable[*Commitment]
	LatestVerifiedCommitment reactive.Variable[*Commitment]
	ClaimedWeight            reactive.Variable[uint64]
	AttestedWeight           reactive.Variable[uint64]
	VerifiedWeight           reactive.Variable[uint64]
	SyncThreshold            reactive.Variable[iotago.SlotIndex]
	WarpSyncThreshold        reactive.Variable[iotago.SlotIndex]
	RequestAttestations      reactive.Variable[bool]
	Engine                   reactive.Variable[*engine.Engine]
	IsSolid                  reactive.Event
	IsEvicted                reactive.Event

	commitments   *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]
	parentEngine  reactive.Variable[*engine.Engine]
	spawnedEngine reactive.Variable[*engine.Engine]
	instantiate   reactive.Variable[bool]

	log.Logger
}

func NewChain(logger log.Logger) *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		RequestAttestations:      reactive.NewVariable[bool](),
		IsEvicted:                reactive.NewEvent(),

		commitments:   shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		parentEngine:  reactive.NewVariable[*engine.Engine](),
		instantiate:   reactive.NewVariable[bool](),
		spawnedEngine: reactive.NewVariable[*engine.Engine](),
	}

	c.ClaimedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestCommitment)
	c.AttestedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestAttestedCommitment)
	c.VerifiedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestVerifiedCommitment)

	c.WarpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitment *Commitment) iotago.SlotIndex {
		if latestCommitment == nil || latestCommitment.Index() < WarpSyncOffset {
			return 0
		}

		return latestCommitment.Index() - WarpSyncOffset
	}, c.LatestCommitment)

	c.SyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitment *Commitment) iotago.SlotIndex {
		if latestVerifiedCommitment == nil {
			return SyncWindow + 1
		}

		return latestVerifiedCommitment.Index() + SyncWindow + 1
	}, c.LatestVerifiedCommitment)

	c.Engine = reactive.NewDerivedVariable2(func(spawnedEngine, parentEngine *engine.Engine) *engine.Engine {
		if spawnedEngine != nil {
			return spawnedEngine
		}

		return parentEngine
	}, c.spawnedEngine, c.parentEngine)

	c.ForkingPoint.OnUpdateWithContext(func(_, forkingPoint *Commitment, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		withinContext(func() func() {
			return forkingPoint.Parent.OnUpdate(func(_, parent *Commitment) {
				withinContext(func() func() {
					return c.parentEngine.InheritFrom(parent.Engine)
				})
			})
		})
	})

	c.Logger = logger.NewEntityLogger("Chain", c.IsEvicted, func(entityLogger log.Logger) {
		logger.LogTrace("created new chain", "name", entityLogger.LogName())

		c.ForkingPoint.LogUpdates(entityLogger, log.LevelDebug, "ForkingPoint", func(commitment *Commitment) string {
			if commitment == nil {
				return "nil"
			}

			return commitment.ID().String()
		})
		c.ClaimedWeight.LogUpdates(entityLogger, log.LevelDebug, "ClaimedWeight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelDebug, "AttestedWeight")
		c.VerifiedWeight.LogUpdates(entityLogger, log.LevelDebug, "VerifiedWeight")
		c.LatestCommitment.LogUpdates(entityLogger, log.LevelTrace, "LatestCommitment", func(commitment *Commitment) string {
			if commitment == nil {
				return "nil"
			}

			return commitment.ID().String()
		})
	})

	return c
}

func (c *Chain) Commitment(index iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c; currentChain != nil; {
		switch forkingPoint := currentChain.ForkingPoint.Get(); {
		case forkingPoint == nil:
			return nil, false // this should never happen, but we can handle it gracefully anyway
		case forkingPoint.Index() == index:
			return forkingPoint, true
		case index > forkingPoint.Index():
			return currentChain.commitments.Get(index)
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

func (c *Chain) InSyncRange(index iotago.SlotIndex) bool {
	if latestVerifiedCommitment := c.LatestVerifiedCommitment.Get(); latestVerifiedCommitment != nil {
		return index > c.LatestVerifiedCommitment.Get().Index() && index < c.SyncThreshold.Get()
	}

	return false
}

func (c *Chain) registerCommitment(commitment *Commitment) (unregister func()) {
	c.commitments.Set(commitment.Index(), commitment)

	// maxCommitment returns the Commitment object with the higher index.
	maxCommitment := func(other *Commitment) *Commitment {
		if commitment == nil || other != nil && other.Index() >= commitment.Index() {
			return other
		}

		return commitment
	}

	c.LatestCommitment.Compute(maxCommitment)

	unsubscribe := lo.Batch(
		commitment.IsAttested.OnTrigger(func() { c.LatestAttestedCommitment.Compute(maxCommitment) }),
		commitment.IsVerified.OnTrigger(func() { c.LatestVerifiedCommitment.Compute(maxCommitment) }),
	)

	return func() {
		unsubscribe()

		c.commitments.Delete(commitment.Index())

		resetToParent := func(latestCommitment *Commitment) *Commitment {
			if latestCommitment == nil || commitment.Index() > latestCommitment.Index() {
				return latestCommitment
			}

			return commitment.Parent.Get()
		}

		c.LatestCommitment.Compute(resetToParent)
		c.LatestAttestedCommitment.Compute(resetToParent)
		c.LatestVerifiedCommitment.Compute(resetToParent)
	}
}
