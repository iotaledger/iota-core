package protocol

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type HeaviestCandidates struct {
	ClaimedWeight *HeaviestCandidate

	AttestedWeight *HeaviestCandidate

	VerifiedWeight *HeaviestCandidate

	measuredSlot reactive.Variable[iotago.SlotIndex]

	reactive.EvictionState[iotago.SlotIndex]
}

func NewHeaviestCandidates() *HeaviestCandidates {
	c := &HeaviestCandidates{
		measuredSlot:   reactive.NewVariable[iotago.SlotIndex](increasing[iotago.SlotIndex]),
		ClaimedWeight:  newHeaviestCandidate((*Commitment).weightAddr),
		AttestedWeight: newHeaviestCandidate((*Commitment).weightAddr),
		VerifiedWeight: newHeaviestCandidate((*Commitment).weightAddr),
	}

	c.measuredSlot.WithNonEmptyValue(c.updateCandidates)

	return c
}

func (c *HeaviestCandidates) Track(chain *Chain) (teardown func()) {
	return chain.LatestCommitment.OnUpdate(func(_ *Commitment, latestCommitment *Commitment) {
		targetSlot := latestCommitment.ID().Index()

		if evictionEvent := c.EvictionEvent(targetSlot); !evictionEvent.WasTriggered() {
			c.ClaimedWeight.registerCommitment(targetSlot, latestCommitment, evictionEvent)
			c.AttestedWeight.registerCommitment(targetSlot, latestCommitment, evictionEvent)
			c.VerifiedWeight.registerCommitment(targetSlot, latestCommitment, evictionEvent)

			c.measuredSlot.Set(targetSlot - measurementOffset)
		}
	})
}

func (c *HeaviestCandidates) updateCandidates(slot iotago.SlotIndex) (teardown func()) {
	return lo.Batch(
		c.ClaimedWeight.update(slot),
		c.AttestedWeight.update(slot),
		c.VerifiedWeight.update(slot),
	)
}

type HeaviestCandidate struct {
	reactive.Variable[*Chain]

	weightedCommitmentsBySlot *shrinkingmap.ShrinkingMap[iotago.SlotIndex, reactive.SortedSet[*Commitment]]

	sortedSetFactory func() reactive.SortedSet[*Commitment]
}

func newHeaviestCandidate(weightVariable func(element *Commitment) reactive.Variable[uint64]) *HeaviestCandidate {
	return &HeaviestCandidate{
		Variable:                  reactive.NewVariable[*Chain](),
		weightedCommitmentsBySlot: shrinkingmap.New[iotago.SlotIndex, reactive.SortedSet[*Commitment]](),
		sortedSetFactory: func() reactive.SortedSet[*Commitment] {
			return reactive.NewSortedSet(weightVariable)
		},
	}
}

func (c *HeaviestCandidate) update(slot iotago.SlotIndex) func() {
	return lo.Return1(c.weightedCommitmentsBySlot.Get(slot)).HeaviestElement().WithNonEmptyValue(func(heaviestCommitment *Commitment) (teardown func()) {
		heaviestChain := heaviestCommitment.Chain.Get()
		if true /* heaviest chain is not equal main chain */ {
			slotsWithHeaviestChain := reactive.NewCounter[*Commitment](func(commitment *Commitment) bool {
				return commitment.Chain.Get() == heaviestChain
			})

			unsubscribeFromCounter := slotsWithHeaviestChain.OnUpdate(func(_ int, slotsWithHeaviestChain int) {
				if iotago.SlotIndex(slotsWithHeaviestChain) >= chainSwitchingThreshold-1 {
					c.Set(heaviestChain)
				} else {
					c.Set(nil)
				}
			})

			teardownFunctions := []func(){unsubscribeFromCounter}
			for i := iotago.SlotIndex(1); i < chainSwitchingThreshold; i++ {
				if commitments, commitmentsExist := c.weightedCommitmentsBySlot.Get(slot - i); commitmentsExist {
					teardownFunctions = append(teardownFunctions, slotsWithHeaviestChain.Monitor(commitments.HeaviestElement()))
				}
			}

			teardown = lo.Batch(teardownFunctions...)
		}

		return teardown
	})
}

func (c *HeaviestCandidate) registerCommitment(slot iotago.SlotIndex, commitment *Commitment, evictionEvent reactive.Event) {
	sortedCommitments, slotCreated := c.weightedCommitmentsBySlot.GetOrCreate(slot, c.sortedSetFactory)
	if slotCreated {
		evictionEvent.OnTrigger(func() { c.weightedCommitmentsBySlot.Delete(slot) })
	}

	sortedCommitments.Add(commitment)
}

const (
	chainSwitchingThreshold iotago.SlotIndex = 10

	measurementOffset iotago.SlotIndex = 1
)
