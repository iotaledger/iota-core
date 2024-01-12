package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainWeights struct {
	LatestSlot reactive.Variable[iotago.SlotIndex]

	WeightReference reactive.Variable[iotago.SlotIndex]

	Claimed *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	Attested *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	Verified *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewChainWeights() *ChainWeights {
	c := &ChainWeights{
		LatestSlot:      reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		WeightReference: reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		Claimed:         shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		Attested:        shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		Verified:        shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
	}

	c.WeightReference.DeriveValueFrom(reactive.NewDerivedVariable(func(_ iotago.SlotIndex, latestSlot iotago.SlotIndex) iotago.SlotIndex {
		return latestSlot - 1
	}, c.LatestSlot))

	return c
}

func (c *ChainWeights) Track(chain *Chain) (teardown func()) {
	return chain.LatestCommitment.OnUpdate(func(_ *Commitment, latestCommitment *Commitment) {
		if targetSlot := latestCommitment.ID().Index(); !c.EvictionEvent(targetSlot).WasTriggered() {
			c.weightedCommitments(c.Claimed, targetSlot).Add(latestCommitment)
			c.weightedCommitments(c.Attested, targetSlot).Add(latestCommitment)
			c.weightedCommitments(c.Verified, targetSlot).Add(latestCommitment)

			c.LatestSlot.Compute(func(currentValue iotago.SlotIndex) iotago.SlotIndex {
				return max(currentValue, targetSlot)
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
