package chainmanagerv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
)

type Chain struct {
	root reactive.Variable[*CommitmentMetadata]

	commitments *ChainCommitments

	weight *ChainWeight

	thresholds *ChainThresholds

	evicted reactive.Event
}

func NewChain(rootCommitment *CommitmentMetadata) *Chain {
	c := &Chain{
		root:    reactive.NewVariable[*CommitmentMetadata]().Init(rootCommitment),
		evicted: reactive.NewEvent(),
	}

	c.commitments = NewChainCommitments(c)
	c.weight = NewChainWeight(c)
	c.thresholds = NewChainThresholds(c)

	rootCommitment.SetChain(c)

	return c
}

func (c *Chain) Root() *CommitmentMetadata {
	return c.root.Get()
}

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

func (c *Chain) Weight() *ChainWeight {
	return c.weight
}

func (c *Chain) Commitments() *ChainCommitments {
	return c.commitments
}

func (c *Chain) Thresholds() *ChainThresholds {
	return c.thresholds
}

func (c *Chain) ReactiveRoot() reactive.Variable[*CommitmentMetadata] {
	return c.root
}
