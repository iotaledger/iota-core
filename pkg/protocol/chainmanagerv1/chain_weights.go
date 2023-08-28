package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

// chainWeights is a reactive component that tracks the cumulative weight of a chain.
type chainWeights struct {
	// chain is the chain that this chainWeights is tracking the cumulative weight of.
	chain *Chain

	// claimed contains the total cumulative weight of the chain that is claimed by the latest commitments.
	claimed reactive.Variable[uint64]

	// attested contains the total cumulative weight of the chain that we received attestations for.
	attested reactive.Variable[uint64]

	// verified contains the total cumulative weight of the chain that we verified ourselves.
	verified reactive.Variable[uint64]
}

// newChainWeights creates a new chainWeights for the given chain.
func newChainWeights(chain *Chain) *chainWeights {
	return &chainWeights{
		chain:    chain,
		claimed:  reactive.NewDerivedVariable[uint64](zeroValueIfNil((*Commitment).CumulativeWeight), chain.LatestCommitmentVariable()),
		attested: reactive.NewDerivedVariable[uint64](zeroValueIfNil((*Commitment).CumulativeWeight), chain.LatestAttestedCommitmentVariable()),
		verified: reactive.NewDerivedVariable[uint64](zeroValueIfNil((*Commitment).CumulativeWeight), chain.LatestVerifiedCommitmentVariable()),
	}
}

// ClaimedWeight returns the total cumulative weight of the chain that is claimed by the latest commitments.
func (c *chainWeights) ClaimedWeight() uint64 {
	return c.claimed.Get()
}

// ClaimedWeightVariable returns a reactive variable that tracks the total cumulative weight of the chain that is claimed by
// the latest commitments.
func (c *chainWeights) ClaimedWeightVariable() reactive.Variable[uint64] {
	return c.claimed
}

// AttestedWeight returns the total cumulative weight of the chain that we received attestations for.
func (c *chainWeights) AttestedWeight() uint64 {
	return c.attested.Get()
}

// AttestedWeightVariable returns a reactive variable that tracks the total cumulative weight of the chain that we received
// attestations for.
func (c *chainWeights) AttestedWeightVariable() reactive.Variable[uint64] {
	return c.attested
}

// VerifiedWeight returns the total cumulative weight of the chain that we verified ourselves.
func (c *chainWeights) VerifiedWeight() uint64 {
	return c.verified.Get()
}

// VerifiedWeightVariable returns a reactive variable that tracks the total cumulative weight of the chain that we verified
// ourselves.
func (c *chainWeights) VerifiedWeightVariable() reactive.Variable[uint64] {
	return c.verified
}
