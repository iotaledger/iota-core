package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

// Chain is a reactive component that manages the state of a chain.
type Chain struct {
	// root contains the Commitment object that spawned this chain.
	root *Commitment

	// evicted is an event that gets triggered when the chain gets evicted.
	evicted reactive.Event

	// chainCommitments is a reactive collection of Commitment objects that belong to the same chain.
	*chainCommitments

	// weight is a reactive component that tracks the cumulative weight of the chain.
	*chainWeights

	// thresholds is a reactive component that tracks the thresholds of the chain.
	*chainThresholds
}

// NewChain creates a new Chain instance.
func NewChain(root *Commitment) *Chain {
	c := &Chain{
		root:    root,
		evicted: reactive.NewEvent(),
	}

	// embed reactive subcomponents
	c.chainCommitments = newChainCommitments(c)
	c.chainWeights = newChainWeights(c)
	c.chainThresholds = newChainThresholds(c)

	// associate the root commitment with its chain
	root.chain.Set(c)

	return c
}

// Root returns the Commitment object that spawned this chain.
func (c *Chain) Root() *Commitment {
	return c.root
}

// ParentChain returns the parent chain of this chain (if it exists and is solid).
func (c *Chain) ParentChain() *Chain {
	root := c.Root()
	if root == nil {
		return nil
	}

	parent := root.Parent()
	if parent == nil {
		return nil
	}

	return parent.Chain()
}

// Evicted returns whether the chain got evicted.
func (c *Chain) Evicted() bool {
	return c.evicted.WasTriggered()
}

// EvictedEvent returns a reactive event that gets triggered when the chain is evicted.
func (c *Chain) EvictedEvent() reactive.Event {
	return c.evicted
}
