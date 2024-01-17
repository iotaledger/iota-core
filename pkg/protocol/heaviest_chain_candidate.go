package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type HeaviestChainCandidate struct {
	reactive.Variable[*Chain]

	chains *Chains

	weightVariable func(element *Commitment) reactive.Variable[uint64]

	weightedCommitmentsBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]
}

func newHeaviestChainCandidate(chains *Chains, weightVariable func(element *Commitment) reactive.Variable[uint64]) *HeaviestChainCandidate {
	return &HeaviestChainCandidate{
		Variable:                  reactive.NewVariable[*Chain](),
		chains:                    chains,
		weightedCommitmentsBySlot: shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		weightVariable:            weightVariable,
	}
}

func (h *HeaviestChainCandidate) measureAt(slot iotago.SlotIndex) (teardown func()) {
	if slot < chainSwitchingThreshold {
		return
	}

	commitments, commitmentsExist := h.weightedCommitmentsBySlot.Get(slot)
	if !commitmentsExist {
		return
	}

	return commitments.HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) func() {
		var teardownFunctions []func()

		if heaviestChain := heaviestCommitment.Chain.Get(); heaviestChain != h.chains.Main.Get() {
			slotsWithHeaviestChain := reactive.NewCounter[*Commitment](func(commitment *Commitment) bool {
				return commitment.Chain.Get() == heaviestChain
			})

			for i := iotago.SlotIndex(1); i < chainSwitchingThreshold; i++ {
				if earlierCommitments, earlierCommitmentsExist := h.weightedCommitmentsBySlot.Get(slot - i); earlierCommitmentsExist {
					teardownFunctions = append(teardownFunctions, slotsWithHeaviestChain.Monitor(earlierCommitments.HeaviestElement()))
				}
			}

			teardownFunctions = append(teardownFunctions, slotsWithHeaviestChain.OnUpdate(func(_ int, slotsWithHeaviestChain int) {
				if iotago.SlotIndex(slotsWithHeaviestChain) >= chainSwitchingThreshold-1 {
					h.Set(heaviestChain)
				} else {
					h.Set(nil)
				}
			}))
		}

		return lo.Batch(teardownFunctions...)
	})
}

func (h *HeaviestChainCandidate) registerCommitment(slot iotago.SlotIndex, commitment *Commitment, evictionEvent reactive.Event) {
	sortedCommitments, slotCreated := h.weightedCommitmentsBySlot.GetOrCreate(slot, func() reactive.SortedSet[*Commitment] {
		return reactive.NewSortedSet(h.weightVariable)
	})

	if slotCreated {
		evictionEvent.OnTrigger(func() { h.weightedCommitmentsBySlot.Delete(slot) })
	}

	sortedCommitments.Add(commitment)
}
