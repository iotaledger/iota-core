package protocol

import (
	"github.com/iotaledger/iota-core/pkg/core/inspection"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (p *Protocol) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(p, func(protocol inspection.InspectedObject) {
		protocol.AddChild("Commitments", p.Commitments)
		protocol.AddChild("Chains", p.Chains)
	}, session...)
}

func (c *Commitments) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(commitments inspection.InspectedObject) {
		commitments.AddChild("Set", c.Set, inspection.InspectSet[*Commitment](c.Set, func(inspectedSet inspection.InspectedObject, element *Commitment) {
			inspectedSet.AddChild(element.LogName(), element)
		}))

		commitments.AddChild("cachedRequests", c.cachedRequests, inspection.InspectMap(c.cachedRequests, func(cachedRequests inspection.InspectedObject, commitmentID iotago.CommitmentID, cachedRequest *promise.Promise[*Commitment]) {
			cachedRequests.AddChild(commitmentID.String(), cachedRequest.Result())
		}))
	}, session...)
}

func (c *Commitment) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(commitment inspection.InspectedObject) {
		commitment.AddChild("Parent", c.Parent.Get())
		commitment.AddChild("MainChild", c.MainChild.Get())
		commitment.AddChild("Chain", c.Chain.Get())
		commitment.AddChild("Children", c.Children, inspection.InspectSet[*Commitment](c.Children, func(children inspection.InspectedObject, child *Commitment) {
			children.AddChild(child.LogName(), child)
		}))
	}, session...)
}

func (c *Chains) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(chains inspection.InspectedObject) {
		chains.AddChild("Main", c.Main.Get())
		chains.AddChild("HeaviestClaimedCandidate", c.HeaviestClaimedCandidate.Get())
		chains.AddChild("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
		chains.AddChild("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
		chains.AddChild("Set", c.Set, inspection.InspectSet[*Chain](c.Set, func(set inspection.InspectedObject, chain *Chain) {
			set.AddChild(chain.LogName(), chain)
		}))
	}, session...)
}

func (c *Chain) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(chain inspection.InspectedObject) {
		chain.AddChild("ForkingPoint", c.ForkingPoint.Get())
		chain.AddChild("ParentChain", c.ParentChain.Get())
		chain.AddChild("ChildChains", c.ChildChains, inspection.InspectSet[*Chain](c.ChildChains, func(childChains inspection.InspectedObject, chain *Chain) {
			childChains.AddChild(chain.LogName(), chain)
		}))
	}, session...)
}
