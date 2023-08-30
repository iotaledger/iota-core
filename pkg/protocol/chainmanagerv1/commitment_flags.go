package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type commitmentFlags struct {
	solid    reactive.Event
	attested reactive.Event
	verified reactive.Event
}

func newCommitmentFlags(commitment *Commitment, isRoot bool) *commitmentFlags {
	c := &commitmentFlags{
		solid:    reactive.NewEvent(),
		attested: reactive.NewEvent(),
		verified: reactive.NewEvent(),
	}

	if isRoot {
		c.solid.Set(true)
		c.attested.Set(true)
		c.verified.Set(true)
	} else {
		commitment.parent.OnUpdate(func(_, parent *Commitment) {
			c.solid.InheritFrom(parent.solid)
		})
	}

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
