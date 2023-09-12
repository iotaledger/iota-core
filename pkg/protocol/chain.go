package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chain is a reactive component that manages the state of a chain.
type Chain struct {
	// forkingPoint contains the Commitment object that spawned this chain.
	ForkingPoint reactive.Variable[*Commitment]

	// latestCommitment is the latest Commitment object in this chain.
	LatestCommitment reactive.Variable[*Commitment]

	// latestAttestedCommitment is the latest attested Commitment object in this chain.
	LatestAttestedCommitment reactive.Variable[*Commitment]

	// latestVerifiedCommitment is the latest verified Commitment object in this chain.
	LatestVerifiedCommitment reactive.Variable[*Commitment]

	// ClaimedWeight contains the total cumulative weight of the chain that is claimed by the latest commitments.
	ClaimedWeight reactive.Variable[uint64]

	// AttestedWeight contains the total cumulative weight of the chain that we received attestations for.
	AttestedWeight reactive.Variable[uint64]

	// VerifiedWeight contains the total cumulative weight of the chain that we verified ourselves.
	VerifiedWeight reactive.Variable[uint64]

	// syncThreshold is the upper bound for slots that are being fed to the engine (to prevent memory exhaustion).
	syncThreshold reactive.Variable[iotago.SlotIndex]

	// warpSyncThreshold defines an offset from latest index where the warp sync process starts (we don't request slots
	// that we are about to commit ourselves).
	warpSyncThreshold reactive.Variable[iotago.SlotIndex]

	// requestAttestations is a flag that indicates whether this chain wants to request attestations.
	requestAttestations reactive.Variable[bool]

	// engine is the engine that is used to process blocks of this chain.
	engine *chainEngine

	// isSolid is an event that gets triggered when the chain becomes solid (all blocks till the forking point of the
	// main chain are available).
	isSolid reactive.Event

	// evicted is an event that gets triggered when the chain gets evicted.
	evicted reactive.Event

	// commitments is a map of Commitment objects that belong to the same chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]
}

// NewChain creates a new Chain instance.
func NewChain() *Chain {
	c := &Chain{
		ForkingPoint:             reactive.NewVariable[*Commitment](),
		commitments:              shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		LatestCommitment:         reactive.NewVariable[*Commitment](),
		LatestAttestedCommitment: reactive.NewVariable[*Commitment](),
		LatestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		requestAttestations:      reactive.NewVariable[bool](),
		evicted:                  reactive.NewEvent(),
	}

	c.engine = newChainEngine(c)

	c.ClaimedWeight = reactive.NewDerivedVariable(cumulativeWeight, c.LatestCommitment)
	c.AttestedWeight = reactive.NewDerivedVariable(cumulativeWeight, c.LatestAttestedCommitment)
	c.VerifiedWeight = reactive.NewDerivedVariable(cumulativeWeight, c.LatestVerifiedCommitment)

	c.warpSyncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestCommitment *Commitment) iotago.SlotIndex {
		if latestCommitment == nil || latestCommitment.Index() < WarpSyncOffset {
			return 0
		}

		return latestCommitment.Index() - WarpSyncOffset
	}, c.LatestCommitment)

	c.syncThreshold = reactive.NewDerivedVariable[iotago.SlotIndex](func(latestVerifiedCommitment *Commitment) iotago.SlotIndex {
		if latestVerifiedCommitment == nil {
			return SyncWindow + 1
		}

		return latestVerifiedCommitment.Index() + SyncWindow + 1
	}, c.LatestVerifiedCommitment)

	return c
}

// Commitment returns the Commitment object with the given index, if it exists.
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

// SyncThreshold returns a reactive variable that contains the upper bound for slots that are being fed to the
// engine (to prevent memory exhaustion).
func (c *Chain) SyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.syncThreshold
}

// WarpSyncThreshold returns a reactive variable that contains an offset from latest index where the warp sync
// process starts (we don't request slots that we are about to commit ourselves).
func (c *Chain) WarpSyncThreshold() reactive.Variable[iotago.SlotIndex] {
	return c.warpSyncThreshold
}

// RequestAttestations returns a reactive variable that contains a flag that indicates whether this chain shall request
// attestations.
func (c *Chain) RequestAttestations() reactive.Variable[bool] {
	return c.requestAttestations
}

func (c *Chain) Engine() *engine.Engine {
	return c.engine.Get()
}

// EngineR returns a reactive variable that contains the engine that is used to process blocks of this chain.
func (c *Chain) EngineR() reactive.Variable[*engine.Engine] {
	return c.engine
}

// InSyncRange returns true if the given index is in the sync range of this chain.
func (c *Chain) InSyncRange(index iotago.SlotIndex) bool {
	if latestVerifiedCommitment := c.LatestVerifiedCommitment.Get(); latestVerifiedCommitment != nil {
		return index > c.LatestVerifiedCommitment.Get().Index() && index < c.syncThreshold.Get()
	}

	return false
}

// Evicted returns a reactive event that gets triggered when the chain is evicted.
func (c *Chain) Evicted() reactive.Event {
	return c.evicted
}

// registerCommitment adds a Commitment object to this collection.
func (c *Chain) registerCommitment(commitment *Commitment) {
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

	// unsubscribe and unregister the commitment when it changes its chain
	commitment.Chain.OnUpdateOnce(func(_, _ *Chain) {
		unsubscribe()

		c.unregisterCommitment(commitment)
	}, func(_, newChain *Chain) bool { return newChain != c })
}

// unregisterCommitment removes a Commitment object from this collection.
func (c *Chain) unregisterCommitment(commitment *Commitment) {
	c.commitments.Delete(commitment.Index())

	resetToParent := func(latestCommitment *Commitment) *Commitment {
		if commitment.Index() > latestCommitment.Index() {
			return latestCommitment
		}

		return commitment.Parent.Get()
	}

	c.LatestCommitment.Compute(resetToParent)
	c.LatestAttestedCommitment.Compute(resetToParent)
	c.LatestVerifiedCommitment.Compute(resetToParent)
}

type chainEngine struct {
	reactive.Variable[*engine.Engine]

	parentEngine reactive.Variable[*engine.Engine]

	spawnedEngine reactive.Variable[*engine.Engine]

	instantiate reactive.Variable[bool]
}

func newChainEngine(chain *Chain) *chainEngine {
	e := &chainEngine{
		parentEngine:  reactive.NewVariable[*engine.Engine](),
		instantiate:   reactive.NewVariable[bool](),
		spawnedEngine: reactive.NewVariable[*engine.Engine](),
	}

	chain.ForkingPoint.OnUpdateWithContext(func(_, forkingPoint *Commitment, withinContext func(subscriptionFactory func() (unsubscribe func()))) {
		withinContext(func() func() {
			return forkingPoint.Parent.OnUpdate(func(_, parent *Commitment) {
				withinContext(func() func() {
					return e.parentEngine.InheritFrom(parent.Engine)
				})
			})
		})
	})

	e.Variable = reactive.NewDerivedVariable2(func(spawnedEngine, parentEngine *engine.Engine) *engine.Engine {
		if spawnedEngine != nil {
			return spawnedEngine
		}

		return parentEngine
	}, e.spawnedEngine, e.parentEngine)

	return e
}

func cumulativeWeight(commitment *Commitment) uint64 {
	if commitment == nil {
		return 0
	}

	return commitment.CumulativeWeight()
}
