package protocol

import "github.com/iotaledger/hive.go/ds/reactive"

type requestAttestations struct {
	reactive.Variable[bool]

	parentAttested reactive.Event

	isDirectlyAboveLatestAttestedCommitment reactive.Variable[bool]
}

func newRequestAttestations(commitment *Commitment) *requestAttestations {
	c := &requestAttestations{
		Variable:       reactive.NewVariable[bool](),
		parentAttested: reactive.NewEvent(),
	}

	c.isDirectlyAboveLatestAttestedCommitment = reactive.NewDerivedVariable2(func(parentAttested, attested bool) bool {
		return parentAttested && !attested
	}, c.parentAttested, commitment.attested)

	commitment.parent.OnUpdateOnce(func(_, parent *Commitment) { c.parentAttested.InheritFrom(parent.attested) })

	var attestationRequestedByChain reactive.DerivedVariable[bool]

	commitment.chain.OnUpdate(func(_, newChain *Chain) {
		// cleanup the old chain specific derived variable if it exists
		if attestationRequestedByChain != nil {
			attestationRequestedByChain.Unsubscribe()
		}

		// create a chain specific derived variable
		attestationRequestedByChain = reactive.NewDerivedVariable2(func(requestAttestations, isDirectlyAboveLatestAttestedCommitment bool) bool {
			return requestAttestations && isDirectlyAboveLatestAttestedCommitment
		}, newChain.requestAttestations, c.isDirectlyAboveLatestAttestedCommitment)

		// expose the chain specific derived variable to the commitment property
		c.InheritFrom(attestationRequestedByChain)
	})

	return c
}
