package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

type commitmentChainSwitchingFlags struct {
	attestationRequested reactive.Variable[bool]
}

func newCommitmentChainSwitchingFlags(commitment *Commitment, isRoot bool) *commitmentChainSwitchingFlags {
	c := &commitmentChainSwitchingFlags{
		attestationRequested: reactive.NewVariable[bool](),
	}

	parentAttested := reactive.NewEvent()
	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		parentAttested.InheritFrom(parent.attested)
	})

	isDirectlyAboveLatestAttestedCommitment := reactive.NewDerivedVariable2(func(parentAttested, attested bool) bool {
		return parentAttested && !attested
	}, parentAttested, commitment.attested)

	var attestationRequestedByChain reactive.DerivedVariable[bool]

	commitment.chain.OnUpdate(func(_, newChain *Chain) {
		// cleanup the old chain specific derived variable if it exists
		if attestationRequestedByChain != nil {
			attestationRequestedByChain.Unsubscribe()
		}

		// create a chain specific derived variable
		attestationRequestedByChain = reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
			return requestAttestations && isDirectlyAboveLatestAttestedCommitment
		}, newChain.requestAttestations, isDirectlyAboveLatestAttestedCommitment)

		// expose the chain specific derived variable to the commitment property
		c.attestationRequested.InheritFrom(attestationRequestedByChain)
	})

	return c
}

func (c *commitmentChainSwitchingFlags) AttestationRequested() reactive.Variable[bool] {
	return c.attestationRequested
}
