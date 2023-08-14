package chainmanager

import (
	"fmt"

	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ChainCommitment struct {
	id         iotago.CommitmentID
	commitment *model.Commitment

	solid       reactive.Event
	mainChildID iotago.CommitmentID
	children    *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *ChainCommitment]
	chain       *Chain

	mutex syncutils.RWMutex
}

func NewChainCommitment(id iotago.CommitmentID) *ChainCommitment {
	return &ChainCommitment{
		id:       id,
		solid:    reactive.NewEvent(),
		children: shrinkingmap.New[iotago.CommitmentID, *ChainCommitment](),
	}
}

func (c *ChainCommitment) ID() iotago.CommitmentID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.id
}

func (c *ChainCommitment) Commitment() *model.Commitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.commitment
}

func (c *ChainCommitment) Children() []*ChainCommitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.children.Values()
}

func (c *ChainCommitment) Chain() (chain *Chain) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.chain
}

func (c *ChainCommitment) Solid() reactive.Event {
	return c.solid
}

func (c *ChainCommitment) PublishCommitment(commitment *model.Commitment) (published bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if published = c.commitment == nil; published {
		c.commitment = commitment
	}

	return
}

func (c *ChainCommitment) registerChild(child *ChainCommitment) (isSolid bool, chain *Chain, wasForked bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.children.Size() == 0 {
		c.mainChildID = child.ID()
	}

	if c.children.Set(child.ID(), child); c.children.Size() > 1 {
		return c.solid.Get(), NewChain(child), true
	}

	return c.solid.Get(), c.chain, false
}

func (c *ChainCommitment) deleteChild(child *ChainCommitment) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.children.Delete(child.ID())
}

func (c *ChainCommitment) mainChild() *ChainCommitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return lo.Return1(c.children.Get(c.mainChildID))
}

func (c *ChainCommitment) setMainChild(commitment *ChainCommitment) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.children.Has(commitment.ID()) {
		return ierrors.Errorf("trying to set a main child %s before registering it as a child", commitment.ID())
	}
	c.mainChildID = commitment.ID()

	return nil
}

func (c *ChainCommitment) publishChain(chain *Chain) (wasPublished bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if wasPublished = c.chain == nil; wasPublished {
		c.chain = chain
	}

	return
}

func (c *ChainCommitment) replaceChain(chain *Chain) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.chain = chain
}

func (c *ChainCommitment) String() string {
	// Generate chainString before locking c.mutex to avoid potential deadlock due to locking ChainCommitment and
	// Chain mutexes in different order across different goroutines.
	chainString := c.Chain().String()

	c.mutex.RLock()
	defer c.mutex.RUnlock()

	builder := stringify.NewStructBuilder("ChainCommitment",
		stringify.NewStructField("ID", c.id),
		stringify.NewStructField("Commitment", c.commitment.String()),
		stringify.NewStructField("Solid", c.solid),
		stringify.NewStructField("Chain", chainString),
		stringify.NewStructField("MainChildID", c.mainChildID),
	)

	for index, child := range c.children.AsMap() {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("children%d", index), child.ID()))
	}

	return builder.String()
}
