package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type HeaviestChainCandidate struct {
	reactive.Variable[*Chain]

	mainChain reactive.ReadableVariable[*Chain]

	weightedCommitmentsBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	weightVariable func(element *Commitment) reactive.Variable[uint64]
}

func newHeaviestChainCandidate(weightVariable func(element *Commitment) reactive.Variable[uint64], mainChain reactive.ReadableVariable[*Chain]) *HeaviestChainCandidate {
	return &HeaviestChainCandidate{
		Variable:                  reactive.NewVariable[*Chain](),
		mainChain:                 mainChain,
		weightedCommitmentsBySlot: shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		weightVariable:            weightVariable,
	}
}

func (h *HeaviestChainCandidate) measureAt(slot iotago.SlotIndex) func() {
	return lo.Return1(h.weightedCommitmentsBySlot.Get(slot)).HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) (teardown func()) {
		var teardownFunctions []func()

		if heaviestChain := heaviestCommitment.Chain.Get(); heaviestChain != h.mainChain.Get() {
			slotsWithHeaviestChain := reactive.NewCounter[*Commitment](func(commitment *Commitment) bool {
				return commitment.Chain.Get() == heaviestChain
			})

			teardownFunctions = append(teardownFunctions, slotsWithHeaviestChain.OnUpdate(func(_ int, slotsWithHeaviestChain int) {
				if iotago.SlotIndex(slotsWithHeaviestChain) >= chainSwitchingThreshold-1 {
					h.Set(heaviestChain)
				} else {
					h.Set(nil)
				}
			}))

			for i := iotago.SlotIndex(1); i < chainSwitchingThreshold; i++ {
				if commitments, commitmentsExist := h.weightedCommitmentsBySlot.Get(slot - i); commitmentsExist {
					teardownFunctions = append(teardownFunctions, slotsWithHeaviestChain.Monitor(commitments.HeaviestElement()))
				}
			}
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
