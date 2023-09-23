package protocol

import (
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/atomic"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/log"
	"github.com/iotaledger/hive.go/runtime/debug"
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
	RequestAttestations      reactive.Variable[bool]
	InstantiateEngine        reactive.Variable[bool]
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
		RequestAttestations:      reactive.NewVariable[bool](),
		IsEvicted:                reactive.NewEvent(),

		commitments:       shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		parentEngine:      reactive.NewVariable[*engine.Engine](),
		InstantiateEngine: reactive.NewVariable[bool](),
		SpawnedEngine:     reactive.NewVariable[*engine.Engine](),
	}

	c.ClaimedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestCommitment)
	c.AttestedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeAttestedWeight, c.LatestAttestedCommitment)
	c.VerifiedWeight = reactive.NewDerivedVariable((*Commitment).cumulativeWeight, c.LatestVerifiedCommitment)

	c.WarpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitment *Commitment) iotago.SlotIndex {
		if latestCommitment == nil || latestCommitment.Index() < WarpSyncOffset {
			return 0
		}

		return latestCommitment.Index() - WarpSyncOffset
	}, c.LatestCommitment)

	c.SyncThreshold = reactive.NewDerivedVariable2[iotago.SlotIndex](func(forkingPoint, latestVerifiedCommitment *Commitment) iotago.SlotIndex {
		if forkingPoint == nil {
			return SyncWindow + 1
		}

		if latestVerifiedCommitment == nil {
			return forkingPoint.Index() + SyncWindow + 1
		}

		return latestVerifiedCommitment.Index() + SyncWindow + 1
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
		logger.LogDebug("chain created", "name", entityLogger.LogName())

		c.ForkingPoint.LogUpdates(entityLogger, log.LevelDebug, "ForkingPoint", (*Commitment).LogName)
		c.ClaimedWeight.LogUpdates(entityLogger, log.LevelTrace, "ClaimedWeight")
		c.AttestedWeight.LogUpdates(entityLogger, log.LevelTrace, "AttestedWeight")
		c.VerifiedWeight.LogUpdates(entityLogger, log.LevelTrace, "VerifiedWeight")
		c.LatestCommitment.LogUpdates(entityLogger, log.LevelDebug, "LatestCommitment", (*Commitment).LogName)
		c.LatestVerifiedCommitment.LogUpdates(entityLogger, log.LevelDebug, "LatestVerifiedCommitment", (*Commitment).LogName)
		c.RequestAttestations.LogUpdates(entityLogger, log.LevelDebug, "RequestAttestations")
		c.InstantiateEngine.LogUpdates(entityLogger, log.LevelDebug, "InstantiateEngine")
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

func (c *Chain) DispatchBlock(block *model.Block, src peer.ID) (err error) {
	if !c.InSyncRange(block.ID().Index()) {
		return ierrors.Errorf("received block is not in sync range of %s", c.LogName())
	}

	engine := c.SpawnedEngine.Get()
	if engine == nil {
		return ierrors.Errorf("received block for %s without engine", c.LogName())
	}

	engine.ProcessBlockFromPeer(block, src)

	c.LogTrace("dispatched block", "block", block.ID(), "src", src)

	return nil
}

func (c *Chain) InSyncRange(index iotago.SlotIndex) bool {
	if latestVerifiedCommitment := c.LatestVerifiedCommitment.Get(); latestVerifiedCommitment != nil {
		return index > c.LatestVerifiedCommitment.Get().Index() && index < c.SyncThreshold.Get()
	}

	forkingPoint := c.ForkingPoint.Get()

	return forkingPoint != nil && (index > forkingPoint.Index()-1 && index < c.SyncThreshold.Get())
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

	var triggered atomic.Int64

	stackTrace := debug.StackTrace(false, 0)

	go func() {
		time.Sleep(5 * time.Second)

		if triggered.Load() == 0 {
			c.LogError("commitment not triggered after 2 seconds", "index", commitment.Index())

			fmt.Println(stackTrace)
		}
	}()

	unsubscribe := lo.Batch(
		commitment.IsAttested.OnTrigger(func() {
			c.LatestAttestedCommitment.Compute(maxCommitment)
		}),
		commitment.IsVerified.OnTrigger(func() { c.LatestVerifiedCommitment.Compute(maxCommitment) }),
	)

	triggered.Store(1)

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
