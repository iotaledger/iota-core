package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainWeights struct {
	WeightReference reactive.Variable[iotago.SlotIndex]

	Claimed *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	Attested *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	Verified *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainWeights() *ChainWeights {
	c := &ChainWeights{
		WeightReference: reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		Claimed:         shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		Attested:        shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		Verified:        shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
	}

	return c
}

func (c *ChainWeights) Track(chain *Chain) (teardown func()) {
	return chain.LatestCommitment.OnUpdate(func(_ *Commitment, latestCommitment *Commitment) {
		if targetSlot := latestCommitment.ID().Index(); !c.EvictionEvent(targetSlot).WasTriggered() {
			c.weightedCommitments(c.Claimed, targetSlot).Add(latestCommitment)
			c.weightedCommitments(c.Attested, targetSlot).Add(latestCommitment)
			c.weightedCommitments(c.Verified, targetSlot).Add(latestCommitment)

			c.WeightReference.Compute(func(currentValue iotago.SlotIndex) iotago.SlotIndex {
				return max(currentValue, targetSlot-1)
			})
		}
	})
}

func (c *ChainWeights) weightedCommitments(targetMap *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]], targetSlot iotago.SlotIndex) reactive.SortedSet[*Commitment] {
	weightedCommitments, slotCreated := targetMap.GetOrCreate(targetSlot, func() reactive.SortedSet[*Commitment] {
		return reactive.NewSortedSet((*Commitment).weightAddr)
	})

	if slotCreated {
		c.EvictionEvent(targetSlot).OnTrigger(func() {
			targetMap.Delete(targetSlot)
		})
	}

	return weightedCommitments
}
