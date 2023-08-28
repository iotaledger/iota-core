package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

// chainCommitments is a reactive collection of Commitment objects that belong to the same chain.
type chainCommitments struct {
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

// newChainCommitments creates a new chainCommitments instance.
func newChainCommitments(chain *Chain) *chainCommitments {
	return &chainCommitments{
		chain:                    chain,
		commitments:              shrinkingmap.New[iotago.SlotIndex, *Commitment](),
		latestCommitment:         reactive.NewVariable[*Commitment](),
		latestAttestedCommitment: reactive.NewVariable[*Commitment](),
		latestVerifiedCommitment: reactive.NewVariable[*Commitment](),
	}
}

// RegisterCommitment adds a Commitment object to this collection.
func (c *chainCommitments) RegisterCommitment(commitment *Commitment) {
	unsubscribe := c.register(commitment)

	commitment.ChainVariable().OnUpdateOnce(func(_, _ *Chain) {
		unsubscribe()

		c.unregister(commitment)
	}, func(_, newChain *Chain) bool { return newChain != c.chain })
}

// Commitment returns the Commitment object with the given index, if it exists.
func (c *chainCommitments) Commitment(index iotago.SlotIndex) (commitment *Commitment, exists bool) {
	for currentChain := c.chain; currentChain != nil; currentChain = currentChain.ParentChain() {
		if root := currentChain.Root(); root != nil && index >= root.Index() {
			return currentChain.commitments.Get(index)
		}
	}

	return nil, false
}

// LatestCommitment returns the latest Commitment object in this collection.
func (c *chainCommitments) LatestCommitment() *Commitment {
	return c.latestCommitment.Get()
}

// LatestCommitmentVariable returns a reactive variable that always contains the latest Commitment object in this
// collection.
func (c *chainCommitments) LatestCommitmentVariable() reactive.Variable[*Commitment] {
	return c.latestCommitment
}

// LatestAttestedCommitment returns the latest attested Commitment object in this collection.
func (c *chainCommitments) LatestAttestedCommitment() *Commitment {
	return c.latestAttestedCommitment.Get()
}

// LatestAttestedCommitmentVariable returns a reactive variable that always contains the latest attested Commitment object
// in this collection.
func (c *chainCommitments) LatestAttestedCommitmentVariable() reactive.Variable[*Commitment] {
	return c.latestAttestedCommitment
}

// LatestVerified returns the latest verified Commitment object in this collection.
func (c *chainCommitments) LatestVerified() *Commitment {
	return c.latestVerifiedCommitment.Get()
}

// LatestVerifiedCommitmentVariable returns a reactive variable that always contains the latest verified Commitment object
// in this collection.
func (c *chainCommitments) LatestVerifiedCommitmentVariable() reactive.Variable[*Commitment] {
	return c.latestVerifiedCommitment
}

// register adds a Commitment object to this collection.
func (c *chainCommitments) register(commitment *Commitment) (unsubscribe func()) {
	c.commitments.Set(commitment.Index(), commitment)

	c.latestCommitment.Compute(commitment.max)

	return lo.Batch(
		commitment.isAttested.OnTrigger(func() { c.latestAttestedCommitment.Compute(commitment.max) }),
		commitment.isVerified.OnTrigger(func() { c.latestVerifiedCommitment.Compute(commitment.max) }),
	)
}

// unregister removes a Commitment object from this collection.
func (c *chainCommitments) unregister(commitment *Commitment) {
	c.commitments.Delete(commitment.Index())

	resetToParent := func(latestCommitment *Commitment) *Commitment {
		if commitment.Index() > latestCommitment.Index() {
			return latestCommitment
		}

		return commitment.Parent()
	}

	c.latestCommitment.Compute(resetToParent)
	c.latestAttestedCommitment.Compute(resetToParent)
	c.latestVerifiedCommitment.Compute(resetToParent)
}
