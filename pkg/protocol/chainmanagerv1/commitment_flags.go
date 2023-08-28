package chainmanagerv1

import "github.com/iotaledger/hive.go/ds/reactive"

type commitmentFlags struct {
	isSolid    reactive.Event
	isAttested reactive.Event
	isVerified reactive.Event
	isEvicted  reactive.Event
}

func newCommitmentFlags(commitment *Commitment) *commitmentFlags {
	c := &commitmentFlags{
		isSolid:    reactive.NewEvent(),
		isAttested: reactive.NewEvent(),
		isVerified: reactive.NewEvent(),
		isEvicted:  reactive.NewEvent(),
	}

	commitment.ParentVariable().OnUpdate(func(_, parent *Commitment) {
		c.isSolid.InheritFrom(parent.isSolid)
	})

	return c
}

func (c *commitmentFlags) IsSolid() bool {
	return c.isSolid.WasTriggered()
}

func (c *commitmentFlags) IsSolidEvent() reactive.Event {
	return c.isSolid
}

func (c *commitmentFlags) IsAttested() bool {
	return c.isAttested.WasTriggered()
}

func (c *commitmentFlags) IsAttestedEvent() reactive.Event {
	return c.isAttested
}

func (c *commitmentFlags) IsVerified() bool {
	return c.isVerified.WasTriggered()
}

func (c *commitmentFlags) IsVerifiedEvent() reactive.Event {
	return c.isVerified
}

func (c *commitmentFlags) IsEvicted() bool {
	return c.isEvicted.WasTriggered()
}

func (c *commitmentFlags) IsEvictedEvent() reactive.Event {
	return c.isEvicted
}
