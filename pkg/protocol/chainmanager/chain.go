package chainmanager

import (
	"sync"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Chain struct {
	ForkingPoint *ChainCommitment

	latestCommitmentIndex iotago.SlotIndex
	commitmentsByIndex    *shrinkingmap.ShrinkingMap[iotago.SlotIndex, *ChainCommitment]

	sync.RWMutex
}

func NewChain(forkingPoint *ChainCommitment) (fork *Chain) {
	forkingPointIndex := forkingPoint.Commitment().Index()

	c := &Chain{
		ForkingPoint:          forkingPoint,
		latestCommitmentIndex: forkingPointIndex,
		commitmentsByIndex:    shrinkingmap.New[iotago.SlotIndex, *ChainCommitment](),
	}

	c.commitmentsByIndex.Set(forkingPointIndex, forkingPoint)

	return c
}

func (c *Chain) IsSolid() (isSolid bool) {
	c.RLock()
	defer c.RUnlock()

	return c.ForkingPoint.IsSolid()
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

	commitmentIndex := commitment.Commitment().Index()
	if commitmentIndex > c.latestCommitmentIndex {
		c.latestCommitmentIndex = commitmentIndex
	}

	c.commitmentsByIndex.Set(commitmentIndex, commitment)
}

func (c *Chain) dropCommitmentsAfter(index iotago.SlotIndex) {
	c.Lock()
	defer c.Unlock()

	for i := index + 1; i <= c.latestCommitmentIndex; i++ {
		c.commitmentsByIndex.Delete(i)
	}

	if index < c.latestCommitmentIndex {
		c.latestCommitmentIndex = index
	}
}
