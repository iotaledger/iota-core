package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type chainManagerChainSwitching struct {
	candidateChain reactive.Variable[*Chain]
}

func newChainManagerChainSwitching(chainManager *ChainManager) *chainManagerChainSwitching {
	c := &chainManagerChainSwitching{
		candidateChain: reactive.NewVariable[*Chain](),
	}

	heaviestClaimedCandidate := reactive.NewVariable[*Chain]()
	heaviestClaimedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			// prevCandidate.attestationsRequested.Set(false)
		}

		// newCandidate.attestationsRequested.Set(true)
	})

	heaviestAttestedCandidate := reactive.NewVariable[*Chain]()
	heaviestAttestedCandidate.OnUpdate(func(prevCandidate, newCandidate *Chain) {
		if prevCandidate != nil {
			// prevCandidate.stateRequested.Set(false)
		}

		// newCandidate.stateRequested.Set(true)
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
		selectHeaviestCandidate(heaviestClaimedCandidate, chain, (*Chain).ClaimedWeight)
		selectHeaviestCandidate(heaviestAttestedCandidate, chain, (*Chain).AttestedWeight)
		selectHeaviestCandidate(c.candidateChain, chain, (*Chain).VerifiedWeight)
	})

	return c
}

func (c *chainManagerChainSwitching) CandidateChain() reactive.Variable[*Chain] {
	return c.candidateChain
}
