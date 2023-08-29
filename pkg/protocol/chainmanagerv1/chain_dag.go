package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// chainDAG is a reactive collection of Commitment objects that belong to the same chain.
type chainDAG struct {
	// chain is the chain that this collection belongs to.
	chain *Chain

	// commitments is a map of Commitment objects that belong to the same chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *Commitment]

	// latestCommitment is the latest Commitment object in this collection.
	latestCommitment reactive.Variable[*Commitment]

	// latestAttestedCommitment is the latest attested Commitment object in this collection.
	latestAttestedCommitment reactive.Variable[*Commitment]

	// latestVerifiedCommitment is the latest verified Commitment object in this collection.
	latestVerifiedCommitment reactive.Variable[*Commitment]
}

// newChainDAG creates a new chainDAG instance.
func newChainDAG(chain *Chain) *chainDAG {
	return &chainDAG{
		chain:                    chain,
		commitments:              shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		latestCommitment:         reactive.NewVariable[*Commitment](),
		latestAttestedCommitment: reactive.NewVariable[*Commitment](),
		latestVerifiedCommitment: reactive.NewVariable[*Commitment](),
	}
}

// Commitment returns the Commitment object with the given index, if it exists.
func (c *chainDAG) Commitment(index iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c.chain; currentChain != nil; currentChain = currentChain.ParentChain() {
		if root := currentChain.Root(); root != nil && index >= root.Index() {
			return currentChain.commitments.Get(index)
		}
	}

	return nil, false
}

// LatestCommitment returns a reactive variable that always contains the latest Commitment object in this
// collection.
func (c *chainDAG) LatestCommitment() reactive.Variable[*Commitment] {
	return c.latestCommitment
}

// LatestAttestedCommitment returns a reactive variable that always contains the latest attested Commitment object
// in this collection.
func (c *chainDAG) LatestAttestedCommitment() reactive.Variable[*Commitment] {
	return c.latestAttestedCommitment
}

// LatestVerifiedCommitment returns a reactive variable that always contains the latest verified Commitment object
// in this collection.
func (c *chainDAG) LatestVerifiedCommitment() reactive.Variable[*Commitment] {
	return c.latestVerifiedCommitment
}

// registerCommitment adds a Commitment object to this collection.
func (c *chainDAG) registerCommitment(commitment *Commitment) {
	c.commitments.Set(commitment.Index(), commitment)

	c.latestCommitment.Compute(commitment.max)

	unsubscribe := lo.Batch(
		commitment.attested.OnTrigger(func() { c.latestAttestedCommitment.Compute(commitment.max) }),
		commitment.verified.OnTrigger(func() { c.latestVerifiedCommitment.Compute(commitment.max) }),
	)

	commitment.chain.OnUpdateOnce(func(_, _ *Chain) {
		unsubscribe()

		c.unregisterCommitment(commitment)
	}, func(_, newChain *Chain) bool { return newChain != c.chain })
}

// unregisterCommitment removes a Commitment object from this collection.
func (c *chainDAG) unregisterCommitment(commitment *Commitment) {
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
