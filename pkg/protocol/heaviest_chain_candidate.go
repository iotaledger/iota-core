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
	chainSwitchingThreshold := h.chains.protocol.APIForSlot(slot).ProtocolParameters().ChainSwitchingThreshold()

	if slot < iotago.SlotIndex(chainSwitchingThreshold) {
		return
	}

	commitments, commitmentsExist := h.weightedCommitmentsBySlot.Get(slot)
	if !commitmentsExist {
		return
	}

	return commitments.HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) func() {
		var teardownFunctions []func()

		if heaviestChain := heaviestCommitment.Chain.Get(); heaviestChain != h.chains.Main.Get() {
			slotsWithSameChain := reactive.NewCounter[*Commitment](func(commitment *Commitment) bool {
				return commitment.Chain.Get() == heaviestChain
			})

			for i := uint8(1); i < chainSwitchingThreshold; i++ {
				if earlierCommitments, earlierCommitmentsExist := h.weightedCommitmentsBySlot.Get(slot - iotago.SlotIndex(i)); earlierCommitmentsExist {
					teardownFunctions = append(teardownFunctions, slotsWithSameChain.Monitor(earlierCommitments.HeaviestElement()))
				}
			}

			teardownFunctions = append(teardownFunctions, slotsWithSameChain.OnUpdate(func(_ int, slotsWithSameChain int) {

				if slotsWithSameChain >= int(chainSwitchingThreshold)-1 {
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
