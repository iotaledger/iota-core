package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

// ChainWeight is a reactive subcomponent of a Chain that tracks the cumulative weight of the chain.
type ChainWeight struct {
	// chain is the chain that this ChainWeight is tracking the cumulative weight of.
	chain *Chain

	// claimed contains the total cumulative weight of the chain that is claimed by the latest commitments.
	claimed reactive.Variable[uint64]

	// attested contains the total cumulative weight of the chain that we received attestations for.
	attested reactive.Variable[uint64]

	// verified contains the total cumulative weight of the chain that we verified ourselves.
	verified reactive.Variable[uint64]
}

// NewChainWeight creates a new ChainWeight for the given chain.
func NewChainWeight(chain *Chain) *ChainWeight {
	return &ChainWeight{
		chain:    chain,
		claimed:  reactive.NewDerivedVariable[uint64](zeroValueIfNil((*CommitmentMetadata).CumulativeWeight), chain.Commitments().ReactiveLatest()),
		attested: reactive.NewDerivedVariable[uint64](zeroValueIfNil((*CommitmentMetadata).CumulativeWeight), chain.Commitments().ReactiveLatestAttested()),
		verified: reactive.NewDerivedVariable[uint64](zeroValueIfNil((*CommitmentMetadata).CumulativeWeight), chain.Commitments().ReactiveLatestVerified()),
	}
}

// Claimed returns the total cumulative weight of the chain that is claimed by the latest commitments.
func (c *ChainWeight) Claimed() uint64 {
	return c.claimed.Get()
}

// Attested returns the total cumulative weight of the chain that we received attestations for.
func (c *ChainWeight) Attested() uint64 {
	return c.attested.Get()
}

// Verified returns the total cumulative weight of the chain that we verified ourselves.
func (c *ChainWeight) Verified() uint64 {
	return c.verified.Get()
}

// ReactiveClaimed returns a reactive variable that tracks the total cumulative weight of the chain that is claimed by
// the latest commitments.
func (c *ChainWeight) ReactiveClaimed() reactive.Variable[uint64] {
	return c.claimed
}

// ReactiveAttested returns a reactive variable that tracks the total cumulative weight of the chain that we received
// attestations for.
func (c *ChainWeight) ReactiveAttested() reactive.Variable[uint64] {
	return c.attested
}

// ReactiveVerified returns a reactive variable that tracks the total cumulative weight of the chain that we verified
// ourselves.
func (c *ChainWeight) ReactiveVerified() reactive.Variable[uint64] {
	return c.verified
}
