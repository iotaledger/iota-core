package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ChainCommitments is a reactive collection of CommitmentMetadata objects that belong to the same chain.
type ChainCommitments struct {
	// chain is the chain that this collection belongs to.
	chain *Chain

	// commitments is a map of CommitmentMetadata objects that belong to the same chain.
	commitments *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *CommitmentMetadata]

	// latest is the latest CommitmentMetadata object in this collection.
	latest reactive.Variable[*CommitmentMetadata]

	// latestAttested is the latest attested CommitmentMetadata object in this collection.
	latestAttested reactive.Variable[*CommitmentMetadata]

	// latestVerified is the latest verified CommitmentMetadata object in this collection.
	latestVerified reactive.Variable[*CommitmentMetadata]
}

// NewChainCommitments creates a new ChainCommitments instance.
func NewChainCommitments(chain *Chain) *ChainCommitments {
	return &ChainCommitments{
		chain:          chain,
		commitments:    shrinkingmap.New[iotago.SlotIndex, *CommitmentMetadata](),
		latest:         reactive.NewVariable[*CommitmentMetadata](),
		latestAttested: reactive.NewVariable[*CommitmentMetadata](),
		latestVerified: reactive.NewVariable[*CommitmentMetadata](),
	}
}

// Chain returns the chain that this collection belongs to.
func (c *ChainCommitments) Chain() *Chain {
	return c.chain
}

// Register adds a CommitmentMetadata object to this collection.
func (c *ChainCommitments) Register(commitment *CommitmentMetadata) {
	unsubscribe := c.register(commitment)

	commitment.Chain().OnUpdateOnce(func(_, _ *Chain) {
		unsubscribe()

		c.unregister(commitment)
	}, func(_, newChain *Chain) bool { return newChain != c.chain })
}

// Get returns the CommitmentMetadata object with the given index, if it exists.
func (c *ChainCommitments) Get(index iotago.SlotIndex) (commitment *CommitmentMetadata, exists bool) {
	parentChain := func(c *Chain) *Chain {
		if root := c.root.Get(); root != nil {
			if parent := root.Parent(); parent != nil {
				return parent.Chain().Get()
			}
		}

		return nil
	}

	for currentChain := c.chain; currentChain != nil; currentChain = parentChain(currentChain) {
		if root := currentChain.ReactiveRoot().Get(); root != nil && index >= root.Index() {
			return currentChain.commitments.Get(index)
		}
	}

	return nil, false
}

// Latest returns the latest CommitmentMetadata object in this collection.
func (c *ChainCommitments) Latest() *CommitmentMetadata {
	return c.latest.Get()
}

// LatestAttested returns the latest attested CommitmentMetadata object in this collection.
func (c *ChainCommitments) LatestAttested() *CommitmentMetadata {
	return c.latestAttested.Get()
}

// LatestVerified returns the latest verified CommitmentMetadata object in this collection.
func (c *ChainCommitments) LatestVerified() *CommitmentMetadata {
	return c.latestVerified.Get()
}

// ReactiveLatest returns a reactive variable that always contains the latest CommitmentMetadata object in this
// collection.
func (c *ChainCommitments) ReactiveLatest() reactive.Variable[*CommitmentMetadata] {
	return c.latest
}

// ReactiveLatestVerified returns a reactive variable that always contains the latest verified CommitmentMetadata object
// in this collection.
func (c *ChainCommitments) ReactiveLatestVerified() reactive.Variable[*CommitmentMetadata] {
	return c.latestVerified
}

// ReactiveLatestAttested returns a reactive variable that always contains the latest attested CommitmentMetadata object
// in this collection.
func (c *ChainCommitments) ReactiveLatestAttested() reactive.Variable[*CommitmentMetadata] {
	return c.latestAttested
}

// register adds a CommitmentMetadata object to this collection.
func (c *ChainCommitments) register(commitment *CommitmentMetadata) (unsubscribe func()) {
	c.commitments.Set(commitment.Index(), commitment)

	c.latest.Compute(commitment.Max)

	return lo.Batch(
		commitment.attested.OnTrigger(func() { c.latestAttested.Compute(commitment.Max) }),
		commitment.verified.OnTrigger(func() { c.latestVerified.Compute(commitment.Max) }),
	)
}

// unregister removes a CommitmentMetadata object from this collection.
func (c *ChainCommitments) unregister(commitment *CommitmentMetadata) {
	c.commitments.Delete(commitment.Index())

	resetToParent := func(latestCommitment *CommitmentMetadata) *CommitmentMetadata {
		if commitment.Index() > latestCommitment.Index() {
			return latestCommitment
		}

		return commitment.Parent()
	}

	c.latest.Compute(resetToParent)
	c.latestAttested.Compute(resetToParent)
	c.latestVerified.Compute(resetToParent)
}
