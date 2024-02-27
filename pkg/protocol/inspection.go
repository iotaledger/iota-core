package protocol

import (
	"github.com/iotaledger/iota-core/pkg/core/inspection"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (p *Protocol) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(p, func(o inspection.InspectedObject) {
		o.AddChild("Commitments", p.Commitments)
		o.AddChild("Chains", p.Chains)
	}, session...)
}

func (c *Commitments) Inspect(session ...inspection.Session) inspection.InspectedObject {
	var (
		inspectSet = func(set inspection.InspectedObject) {
			c.Range(func(commitment *Commitment) {
				set.AddChild(commitment.LogName(), commitment)
			})
		}

		inspectCachedRequests = func(cachedRequests inspection.InspectedObject) {
			c.cachedRequests.ForEach(func(commitmentID iotago.CommitmentID, cachedRequest *promise.Promise[*Commitment]) bool {
				cachedRequests.AddChild(commitmentID.String(), cachedRequest.Result())

				return true
			})
		}
	)

	return inspection.NewInspectedObject(c, func(o inspection.InspectedObject) {
		o.AddChild("Set", c.Set, inspectSet)
		o.AddChild("cachedRequests", c.cachedRequests, inspectCachedRequests)
	}, session...)
}

func (c *Commitment) Inspect(session ...inspection.Session) inspection.InspectedObject {
	inspectChildren := func(children inspection.InspectedObject) {
		c.Children.Range(func(child *Commitment) {
			children.AddChild(child.LogName(), child)
		})
	}

	return inspection.NewInspectedObject(c, func(o inspection.InspectedObject) {
		o.AddChild("Parent", c.Parent.Get())
		o.AddChild("MainChild", c.MainChild.Get())
		o.AddChild("Chain", c.Chain.Get())
		o.AddChild("Children", c.Children, inspectChildren)
	}, session...)
}

func (c *Chains) Inspect(session ...inspection.Session) inspection.InspectedObject {
	inspectSet := func(set inspection.InspectedObject) {
		c.Set.Range(func(chain *Chain) {
			set.AddChild(chain.LogName(), chain)
		})
	}

	return inspection.NewInspectedObject(c, func(o inspection.InspectedObject) {
		o.AddChild("Set", c.Set, inspectSet)
		o.AddChild("Main", c.Main.Get())
		o.AddChild("HeaviestClaimedCandidate", c.HeaviestClaimedCandidate.Get())
		o.AddChild("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
		o.AddChild("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
	}, session...)
}

func (c *Chain) Inspect(session ...inspection.Session) inspection.InspectedObject {
	inspectChildChains := func(childChains inspection.InspectedObject) {
		c.ChildChains.Range(func(childChain *Chain) {
			childChains.AddChild(childChain.LogName(), childChain)
		})
	}

	return inspection.NewInspectedObject(c, func(chain inspection.InspectedObject) {
		chain.AddChild("ForkingPoint", c.ForkingPoint.Get())
		chain.AddChild("ParentChain", c.ParentChain.Get())
		chain.AddChild("ChildChains", c.ChildChains, inspectChildChains)
	}, session...)
}
