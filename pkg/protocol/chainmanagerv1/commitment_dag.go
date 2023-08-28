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

	c.chain.OnUpdate(func(_, chain *Chain) { chain.RegisterCommitment(commitment) })

	return c
}

func (c *commitmentDAG) Parent() *Commitment {
	return c.parent.Get()
}

func (c *commitmentDAG) Successor() *Commitment {
	return c.successor.Get()
}

func (c *commitmentDAG) SpawnedChain() *Chain {
	return c.spawnedChain.Get()
}

func (c *commitmentDAG) Chain() *Chain {
	return c.chain.Get()
}

func (c *commitmentDAG) ParentVariable() reactive.Variable[*Commitment] {
	return c.parent
}

func (c *commitmentDAG) SuccessorVariable() reactive.Variable[*Commitment] {
	return c.successor
}

func (c *commitmentDAG) SpawnedChainVariable() reactive.Variable[*Chain] {
	return c.spawnedChain
}

func (c *commitmentDAG) ChainVariable() reactive.Variable[*Chain] {
	return c.chain
}

func (c *commitmentDAG) setParent(parent *Commitment) {
	c.parent.Compute(func(currentParent *Commitment) *Commitment {
		if currentParent != nil {
			panic("parent may not be changed once it was set")
		}

		parent.registerChild(c.commitment, c.inheritChain(parent))

		return parent
	})
}

func (c *commitmentDAG) setChain(chain *Chain) {
	c.chain.Set(chain)
}

func (c *commitmentDAG) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	c.successor.Compute(func(currentSuccessor *Commitment) *Commitment {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	unsubscribe := c.successor.OnUpdate(onSuccessorUpdated)

	c.commitment.isEvicted.OnTrigger(unsubscribe)
}

func (c *commitmentDAG) inheritChain(parent *Commitment) func(*Commitment, *Commitment) {
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

				unsubscribeFromParent = parent.chain.OnUpdate(func(_, chain *Chain) { c.setChain(chain) })

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
