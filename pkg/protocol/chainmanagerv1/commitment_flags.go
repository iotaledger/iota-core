package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type commitmentFlags struct {
	solid    reactive.Event
	attested reactive.Event
	verified reactive.Event
	evicted  reactive.Event
}

func newCommitmentFlags(commitment *Commitment) *commitmentFlags {
	c := &commitmentFlags{
		solid:    reactive.NewEvent(),
		attested: reactive.NewEvent(),
		verified: reactive.NewEvent(),
		evicted:  reactive.NewEvent(),
	}

	commitment.ParentVariable().OnUpdate(func(_, parent *Commitment) {
		c.solid.InheritFrom(parent.solid)
	})

	return c
}

func (c *commitmentFlags) Solid() reactive.Event {
	return c.solid
}

func (c *commitmentFlags) Attested() reactive.Event {
	return c.attested
}

func (c *commitmentFlags) Verified() reactive.Event {
	return c.verified
}

func (c *commitmentFlags) Evicted() reactive.Event {
	return c.evicted
}
