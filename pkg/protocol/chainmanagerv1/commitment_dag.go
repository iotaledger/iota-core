package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
)

type commitmentDAG struct {
	commitment   *Commitment
	parent       reactive.Variable[*Commitment]
	successor    reactive.Variable[*Commitment]
	spawnedChain reactive.Variable[*Chain]
	chain        reactive.Variable[*Chain]
}

func newCommitmentDAG(commitment *Commitment) *commitmentDAG {
	c := &commitmentDAG{
		commitment:   commitment,
		parent:       reactive.NewVariable[*Commitment](),
		successor:    reactive.NewVariable[*Commitment](),
		spawnedChain: reactive.NewVariable[*Chain](),
		chain:        reactive.NewVariable[*Chain](),
	}

	c.chain.OnUpdate(func(_, chain *Chain) { chain.registerCommitment(commitment) })

	return c
}

func (c *commitmentDAG) Parent() reactive.Variable[*Commitment] {
	return c.parent
}

func (c *commitmentDAG) Successor() reactive.Variable[*Commitment] {
	return c.successor
}

func (c *commitmentDAG) SpawnedChain() reactive.Variable[*Chain] {
	return c.spawnedChain
}

func (c *commitmentDAG) Chain() reactive.Variable[*Chain] {
	return c.chain
}

func (c *commitmentDAG) setParent(parent *Commitment) {
	c.parent.Compute(func(currentParent *Commitment) *Commitment {
		if currentParent != nil {
			panic("parent may not be changed once it was set")
		}

		parent.registerChild(c.commitment, c.createChainUpdater(parent))

		return parent
	})
}

func (c *commitmentDAG) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	c.successor.Compute(func(currentSuccessor *Commitment) *Commitment {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	unsubscribe := c.successor.OnUpdate(onSuccessorUpdated)

	c.commitment.evicted.OnTrigger(unsubscribe)
}

func (c *commitmentDAG) createChainUpdater(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, successor *Commitment) {
		c.spawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			if successor == nil {
				panic("successor may not be changed back to nil")
			}

			if successor == c.commitment {
				if spawnedChain != nil {
					spawnedChain.evicted.Trigger()
				}

				unsubscribeFromParent = parent.chain.OnUpdate(func(_, chain *Chain) { c.chain.Set(chain) })

				return nil
			}

			if spawnedChain != nil {
				return spawnedChain
			}

			if unsubscribeFromParent != nil {
				unsubscribeFromParent()
			}

			return NewChain(c.commitment)
		})
	}
}
