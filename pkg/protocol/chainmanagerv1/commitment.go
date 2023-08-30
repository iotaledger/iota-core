package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/model"
)

type Commitment struct {
	*model.Commitment

	parent       reactive.Variable[*Commitment]
	successor    reactive.Variable[*Commitment]
	spawnedChain reactive.Variable[*Chain]
	chain        reactive.Variable[*Chain]
	evicted      reactive.Event

	*commitmentFlags
	*commitmentDispatcherFlags
}

func NewCommitment(commitment *model.Commitment) *Commitment {
	c := &Commitment{
		Commitment:   commitment,
		parent:       reactive.NewVariable[*Commitment](),
		successor:    reactive.NewVariable[*Commitment](),
		spawnedChain: reactive.NewVariable[*Chain](),
		chain:        reactive.NewVariable[*Chain](),
		evicted:      reactive.NewEvent(),
	}

	c.commitmentFlags = newCommitmentFlags(c)
	c.commitmentDispatcherFlags = newCommitmentDispatcherFlags(c)

	c.chain.OnUpdate(func(_, chain *Chain) { chain.registerCommitment(c) })

	return c
}

func NewRootCommitment(commitment *model.Commitment) *Commitment {
	commitmentMetadata := NewCommitment(commitment)

	commitmentMetadata.solid.Set(true)
	commitmentMetadata.verified.Set(true)
	commitmentMetadata.isBelowSyncThreshold.Set(true)
	commitmentMetadata.isBelowWarpSyncThreshold.Set(true)
	commitmentMetadata.evicted.Set(false)

	return commitmentMetadata
}

func (c *Commitment) Parent() reactive.Variable[*Commitment] {
	return c.parent
}

func (c *Commitment) Successor() reactive.Variable[*Commitment] {
	return c.successor
}

func (c *Commitment) SpawnedChain() reactive.Variable[*Chain] {
	return c.spawnedChain
}

func (c *Commitment) Chain() reactive.Variable[*Chain] {
	return c.chain
}

func (c *Commitment) Evicted() reactive.Event {
	return c.evicted
}

func (c *Commitment) setParent(parent *Commitment) {
	c.parent.Compute(func(currentParent *Commitment) *Commitment {
		if currentParent != nil {
			panic("parent may not be changed once it was set")
		}

		parent.registerChild(c, c.chainUpdater(parent))

		return parent
	})
}

func (c *Commitment) registerChild(newChild *Commitment, onSuccessorUpdated func(*Commitment, *Commitment)) {
	c.successor.Compute(func(currentSuccessor *Commitment) *Commitment {
		return lo.Cond(currentSuccessor != nil, currentSuccessor, newChild)
	})

	unsubscribe := c.successor.OnUpdate(onSuccessorUpdated)

	c.evicted.OnTrigger(unsubscribe)
}

func (c *Commitment) chainUpdater(parent *Commitment) func(*Commitment, *Commitment) {
	var unsubscribeFromParent func()

	return func(_, successor *Commitment) {
		c.spawnedChain.Compute(func(spawnedChain *Chain) (newSpawnedChain *Chain) {
			if successor == nil {
				panic("successor may not be changed back to nil")
			}

			if successor == c {
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

			return NewChain(c)
		})
	}
}

// max compares the commitment with the given other commitment and returns the one with the higher index.
func (c *Commitment) max(other *Commitment) *Commitment {
	if c == nil || other != nil && other.Index() >= c.Index() {
		return other
	}

	return c
}
