package chainmanager

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chain struct {
	ForkingPoint *ChainCommitment

	latestCommitmentIndex iotago.SlotIndex
	commitmentsByIndex    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *ChainCommitment]

	syncutils.RWMutex
}

func NewChain(forkingPoint *ChainCommitment) (fork *Chain) {
	forkingPointIndex := forkingPoint.Commitment().Slot()

	c := &Chain{
		ForkingPoint:          forkingPoint,
		latestCommitmentIndex: forkingPointIndex,
		commitmentsByIndex:    shrinkingmap.New[iotago.SlotIndex, *ChainCommitment](),
	}

	c.commitmentsByIndex.Set(forkingPointIndex, forkingPoint)

	return c
}

func (c *Chain) SolidEvent() reactive.Event {
	return c.ForkingPoint.SolidEvent()
}

func (c *Chain) Commitment(index iotago.SlotIndex) (commitment *ChainCommitment) {
	c.RLock()
	defer c.RUnlock()

	return lo.Return1(c.commitmentsByIndex.Get(index))
}

func (c *Chain) Size() int {
	c.RLock()
	defer c.RUnlock()

	return c.commitmentsByIndex.Size()
}

func (c *Chain) LatestCommitment() *ChainCommitment {
	c.RLock()
	defer c.RUnlock()

	return lo.Return1(c.commitmentsByIndex.Get(c.latestCommitmentIndex))
}

func (c *Chain) addCommitment(commitment *ChainCommitment) {
	c.Lock()
	defer c.Unlock()

	commitmentIndex := commitment.Commitment().Slot()
	if commitmentIndex > c.latestCommitmentIndex {
		c.latestCommitmentIndex = commitmentIndex
	}

	c.commitmentsByIndex.Set(commitmentIndex, commitment)
}

func (c *Chain) String() string {
	c.RLock()
	defer c.RUnlock()

	builder := stringify.NewStructBuilder("Chain",
		stringify.NewStructField("ForkingPoint", c.ForkingPoint.id),
		stringify.NewStructField("LatestCommitmentIndex", c.latestCommitmentIndex),
	)

	return builder.String()
}
