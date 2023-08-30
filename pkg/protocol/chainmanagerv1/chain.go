package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Chain is a reactive component that manages the state of a chain.
type Chain struct {
	// root contains the Commitment object that spawned this chain.
	root *Commitment

	// commitments is a map of Commitment objects that belong to the same chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	// latestCommitment is the latest Commitment object in this collection.
	latestCommitment reactive.Variable[*Commitment]

	// latestAttestedCommitment is the latest attested Commitment object in this collection.
	latestAttestedCommitment reactive.Variable[*Commitment]

	// latestVerifiedCommitment is the latest verified Commitment object in this collection.
	latestVerifiedCommitment reactive.Variable[*Commitment]

	// evicted is an event that gets triggered when the chain gets evicted.
	evicted reactive.Event

	// weight is a reactive component that tracks the cumulative weight of the chain.
	*chainWeights

	// thresholds is a reactive component that tracks the thresholds of the chain.
	*chainThresholds
}

// NewChain creates a new Chain instance.
func NewChain(root *Commitment) *Chain {
	c := &Chain{
		root:                     root,
		commitments:              shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		latestCommitment:         reactive.NewVariable[*Commitment](),
		latestAttestedCommitment: reactive.NewVariable[*Commitment](),
		latestVerifiedCommitment: reactive.NewVariable[*Commitment](),
		evicted:                  reactive.NewEvent(),
	}

	// embed reactive subcomponents
	c.chainWeights = newChainWeights(c)
	c.chainThresholds = newChainThresholds(c)

	// associate the root commitment with its chain
	root.chain.Set(c)

	return c
}

// Root returns the Commitment object that spawned this chain.
func (c *Chain) Root() *Commitment {
	return c.root
}

// Commitment returns the Commitment object with the given index, if it exists.
func (c *Chain) Commitment(index iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c; currentChain != nil; {
		switch root := currentChain.Root(); {
		case root == nil:
			return nil, false // this should never happen, but we can handle it gracefully anyway
		case root.Index() == index:
			return root, true
		case index > root.Index():
			return currentChain.commitments.Get(index)
		default:
			parent := root.parent.Get()
			if parent == nil {
				return nil, false
			}

			currentChain = parent.chain.Get()
		}
	}

	return nil, false
}

// LatestCommitment returns a reactive variable that always contains the latest Commitment object in this
// collection.
func (c *Chain) LatestCommitment() reactive.Variable[*Commitment] {
	return c.latestCommitment
}

// LatestAttestedCommitment returns a reactive variable that always contains the latest attested Commitment object
// in this collection.
func (c *Chain) LatestAttestedCommitment() reactive.Variable[*Commitment] {
	return c.latestAttestedCommitment
}

// LatestVerifiedCommitment returns a reactive variable that always contains the latest verified Commitment object
// in this collection.
func (c *Chain) LatestVerifiedCommitment() reactive.Variable[*Commitment] {
	return c.latestVerifiedCommitment
}

// Evicted returns a reactive event that gets triggered when the chain is evicted.
func (c *Chain) Evicted() reactive.Event {
	return c.evicted
}

// registerCommitment adds a Commitment object to this collection.
func (c *Chain) registerCommitment(commitment *Commitment) {
	c.commitments.Set(commitment.Index(), commitment)

	c.latestCommitment.Compute(commitment.max)

	unsubscribe := lo.Batch(
		commitment.attested.OnTrigger(func() { c.latestAttestedCommitment.Compute(commitment.max) }),
		commitment.verified.OnTrigger(func() { c.latestVerifiedCommitment.Compute(commitment.max) }),
	)

	commitment.chain.OnUpdateOnce(func(_, _ *Chain) {
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

		return commitment.parent.Get()
	}

	c.latestCommitment.Compute(resetToParent)
	c.latestAttestedCommitment.Compute(resetToParent)
	c.latestVerifiedCommitment.Compute(resetToParent)
}
