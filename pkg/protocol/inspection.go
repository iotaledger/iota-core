package protocol

import (
	"github.com/iotaledger/hive.go/runtime/inspection"
	"github.com/iotaledger/iota-core/pkg/core/promise"
	iotago "github.com/iotaledger/iota.go/v4"
)

// Inspect inspects the protocol and its subcomponents.
func (p *Protocol) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(p, func(protocol inspection.InspectedObject) {
		protocol.Add("Commitments", p.Commitments)
		protocol.Add("Chains", p.Chains)
		protocol.Add("Engines", p.Engines)
	}, session...)
}

// Inspect inspects the Commitments and its subcomponents.
func (c *Commitments) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(commitments inspection.InspectedObject) {
		commitments.Add("Root", c.Root.Get())
		commitments.Add("Set", c.Set, inspection.SetInspector[*Commitment](c.Set, func(inspectedSet inspection.InspectedObject, element *Commitment) {
			inspectedSet.Add(element.LogName(), element)
		}))
		commitments.Add("cachedRequests", c.cachedRequests, inspection.MapInspector(c.cachedRequests, func(cachedRequests inspection.InspectedObject, commitmentID iotago.CommitmentID, cachedRequest *promise.Promise[*Commitment]) {
			cachedRequests.Add(commitmentID.String(), cachedRequest.Result())
		}))
	}, session...)
}

// Inspect inspects the Commitment and its subcomponents.
func (c *Commitment) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(commitment inspection.InspectedObject) {
		commitment.Add("Parent", c.Parent.Get())
		commitment.Add("MainChild", c.MainChild.Get())
		commitment.Add("Chain", c.Chain.Get())
		commitment.Add("Children", c.Children, inspection.SetInspector[*Commitment](c.Children, func(children inspection.InspectedObject, child *Commitment) {
			children.Add(child.LogName(), child)
		}))
	}, session...)
}

// Inspect inspects the Chains and its subcomponents.
func (c *Chains) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(chains inspection.InspectedObject) {
		chains.Add("Main", c.Main.Get())
		chains.Add("HeaviestClaimedCandidate", c.HeaviestClaimedCandidate.Get())
		chains.Add("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
		chains.Add("HeaviestAttestedCandidate", c.HeaviestAttestedCandidate.Get())
		chains.Add("Set", c.Set, inspection.SetInspector[*Chain](c.Set, func(set inspection.InspectedObject, chain *Chain) {
			set.Add(chain.LogName(), chain)
		}))
	}, session...)
}

// Inspect inspects the Chain and its subcomponents.
func (c *Chain) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(c, func(chain inspection.InspectedObject) {
		chain.Add("ForkingPoint", c.ForkingPoint.Get())
		chain.Add("ParentChain", c.ParentChain.Get())
		chain.Add("LatestCommitment", c.LatestCommitment.Get())
		chain.Add("LatestAttestedCommitment", c.LatestAttestedCommitment.Get())
		chain.Add("LatestProducedCommitment", c.LatestProducedCommitment.Get())
		chain.Add("Engine", c.Engine.Get())
		chain.Add("ChildChains", c.ChildChains, inspection.SetInspector[*Chain](c.ChildChains, func(childChains inspection.InspectedObject, chain *Chain) {
			childChains.Add(chain.LogName(), chain)
		}))
		chain.Add("commitments", c.commitments, inspection.MapInspector(c.commitments, func(commitments inspection.InspectedObject, slot iotago.SlotIndex, commitment *Commitment) {
			commitments.Add(slot.String(), commitment)
		}))
	}, session...)
}

// Inspect inspects the Engines and its subcomponents.
func (e *Engines) Inspect(session ...inspection.Session) inspection.InspectedObject {
	return inspection.NewInspectedObject(e, func(engines inspection.InspectedObject) {
		engines.Add("Main", e.Main.Get())
	}, session...)
}
