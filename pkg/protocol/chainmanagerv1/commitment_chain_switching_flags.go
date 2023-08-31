package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

type commitmentChainSwitchingFlags struct {
	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
	attestationRequested                    reactive.Variable[bool]
}

func newCommitmentChainSwitchingFlags(commitment *Commitment, isRoot bool) *commitmentChainSwitchingFlags {
	c := &commitmentChainSwitchingFlags{
		isDirectlyAboveLatestAttestedCommitment: isDirectlyAboveLatestAttestedCommitment(commitment),
		attestationRequested:                    reactive.NewVariable[bool](),
	}

	var attestationRequestedByChain reactive.DerivedVariable[bool]
	commitment.chain.OnUpdate(func(_, newChain *Chain) {
		if attestationRequestedByChain != nil {
			attestationRequestedByChain.Unsubscribe()
		}

		attestationRequestedByChain = reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
			return requestAttestations && isDirectlyAboveLatestAttestedCommitment
		}, newChain.requestAttestations, c.isDirectlyAboveLatestAttestedCommitment)

		c.attestationRequested.InheritFrom(attestationRequestedByChain)
	})

	return c
}

func (c *commitmentChainSwitchingFlags) IsDirectlyAboveLatestAttestedCommitment() reactive.Variable[bool] {
	return c.isDirectlyAboveLatestAttestedCommitment
}

func (c *commitmentChainSwitchingFlags) AttestationRequested() reactive.Variable[bool] {
	return c.attestationRequested
}

func isDirectlyAboveLatestAttestedCommitment(commitment *Commitment) reactive.Variable[bool] {
	parentAttested := reactive.NewEvent()
	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) {
		parentAttested.InheritFrom(parent.attested)
	})

	return reactive.NewDerivedVariable2(func(parentAttested, attested bool) bool {
		return parentAttested && !attested
	}, parentAttested, commitment.attested)
}
