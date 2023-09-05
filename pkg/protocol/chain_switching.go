package protocol

import "github.com/iotaledger/hive.go/ds/reactive"

type ChainSwitching struct {
	heaviestClaimedCandidate reactive.Variable[*Chain]

	heaviestAttestedCandidate reactive.Variable[*Chain]

	heaviestVerifiedCandidate reactive.Variable[*Chain]
}

func NewChainSwitching(chainManager *Chains) *ChainSwitching {
	c := &ChainSwitching{
		heaviestClaimedCandidate:  reactive.NewVariable[*Chain](),
		heaviestAttestedCandidate: reactive.NewVariable[*Chain](),
		heaviestVerifiedCandidate: reactive.NewVariable[*Chain](),
	}

	c.heaviestClaimedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.requestAttestations.Set(false)
		}

		newCandidate.requestAttestations.Set(true)
	})

	c.heaviestAttestedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			prevCandidate.engine.instantiate.Set(false)
		}

		newCandidate.engine.instantiate.Set(true)
	})

	selectHeaviestCandidate := func(candidate reactive.Variable[*Chain], newCandidate *Chain, chainWeight func(*Chain) reactive.Variable[uint64]) {
		chainWeight(newCandidate).OnUpdate(func(_, newChainWeight uint64) {
			if newChainWeight <= chainManager.mainChain.Get().verifiedWeight.Get() {
				return
			}

			candidate.Compute(func(currentCandidate *Chain) *Chain {
				if currentCandidate == nil || currentCandidate.evicted.WasTriggered() || newChainWeight > chainWeight(currentCandidate).Get() {
					return newCandidate
				}

				return currentCandidate
			})
		})
	}

	chainManager.OnChainCreated(func(chain *Chain) {
		// TODO: ON SOLID
		selectHeaviestCandidate(c.heaviestClaimedCandidate, chain, (*Chain).ClaimedWeight)
		selectHeaviestCandidate(c.heaviestAttestedCandidate, chain, (*Chain).AttestedWeight)
		selectHeaviestCandidate(c.heaviestVerifiedCandidate, chain, (*Chain).VerifiedWeight)
	})

	return c
}

func (c *ChainSwitching) HeaviestClaimedCandidate() reactive.Variable[*Chain] {
	return c.heaviestClaimedCandidate
}

func (c *ChainSwitching) HeaviestAttestedCandidate() reactive.Variable[*Chain] {
	return c.heaviestAttestedCandidate
}

func (c *ChainSwitching) HeaviestVerifiedCandidate() reactive.Variable[*Chain] {
	return c.heaviestVerifiedCandidate
}
