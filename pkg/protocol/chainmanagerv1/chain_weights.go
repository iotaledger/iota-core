package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

// chainWeights is a reactive component that tracks the cumulative weight of a chain.
type chainWeights struct {
	// claimedWeight contains the total cumulative weight of the chain that is claimed by the latest commitments.
	claimedWeight reactive.Variable[uint64]

	// attestedWeight contains the total cumulative weight of the chain that we received attestations for.
	attestedWeight reactive.Variable[uint64]

	// verifiedWeight contains the total cumulative weight of the chain that we verified ourselves.
	verifiedWeight reactive.Variable[uint64]
}

// newChainWeights creates a new chainWeights for the given chain.
func newChainWeights(chain *Chain) *chainWeights {
	return &chainWeights{
		claimedWeight:  reactive.NewDerivedVariable[uint64](noPanicIfNil((*Commitment).CumulativeWeight), chain.latestCommitment),
		attestedWeight: reactive.NewDerivedVariable[uint64](noPanicIfNil((*Commitment).CumulativeWeight), chain.latestAttestedCommitment),
		verifiedWeight: reactive.NewDerivedVariable[uint64](noPanicIfNil((*Commitment).CumulativeWeight), chain.latestVerifiedCommitment),
	}
}

// ClaimedWeight returns a reactive variable that tracks the total cumulative weight of the chain that is claimed by
// the latest commitments.
func (c *chainWeights) ClaimedWeight() reactive.Variable[uint64] {
	return c.claimedWeight
}

// AttestedWeight returns a reactive variable that tracks the total cumulative weight of the chain that we received
// attestations for.
func (c *chainWeights) AttestedWeight() reactive.Variable[uint64] {
	return c.attestedWeight
}

// VerifiedWeight returns a reactive variable that tracks the total cumulative weight of the chain that we verified
// ourselves.
func (c *chainWeights) VerifiedWeight() reactive.Variable[uint64] {
	return c.verifiedWeight
}
