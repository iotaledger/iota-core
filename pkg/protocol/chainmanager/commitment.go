package chainmanager

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Commitment struct {
	id         iotago.CommitmentID
	commitment *iotago.Commitment

	solid       bool
	mainChildID iotago.CommitmentID
	children    *shrinkingmap.ShrinkingMap[iotago.CommitmentID, *Commitment]
	chain       *Chain

	mutex sync.RWMutex
}

func NewCommitment(id iotago.CommitmentID) *Commitment {
	return &Commitment{
		id:       id,
		children: shrinkingmap.New[iotago.CommitmentID, *Commitment](),
	}
}

func (c *Commitment) ID() iotago.CommitmentID {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.id
}

func (c *Commitment) Commitment() *iotago.Commitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.commitment
}

func (c *Commitment) Children() []*Commitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.children.Values()
}

func (c *Commitment) Chain() (chain *Chain) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.chain
}

func (c *Commitment) IsSolid() (isSolid bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return c.solid
}

func (c *Commitment) SetSolid(solid bool) (updated bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if updated = c.solid != solid; updated {
		c.solid = solid
	}

	return
}

func (c *Commitment) PublishCommitment(commitment *iotago.Commitment) (published bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if published = c.commitment == nil; published {
		c.commitment = commitment
	}

	return
}

func (c *Commitment) registerChild(child *Commitment) (isSolid bool, chain *Chain, wasForked bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.children.Size() == 0 {
		c.mainChildID = child.ID()
	}

	if c.children.Set(child.ID(), child); c.children.Size() > 1 {
		return c.solid, NewChain(child), true
	}

	return c.solid, c.chain, false
}

func (c *Commitment) deleteChild(child *Commitment) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.children.Delete(child.ID())
}

func (c *Commitment) mainChild() *Commitment {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return lo.Return1(c.children.Get(c.mainChildID))
}

func (c *Commitment) setMainChild(commitment *Commitment) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.children.Has(commitment.ID()) {
		return errors.Errorf("trying to set a main child %s before registering it as a child", commitment.ID())
	}
	c.mainChildID = commitment.ID()

	return nil
}

func (c *Commitment) publishChain(chain *Chain) (wasPublished bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if wasPublished = c.chain == nil; wasPublished {
		c.chain = chain
	}

	return
}

func (c *Commitment) replaceChain(chain *Chain) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.chain = chain
}

func (c *Commitment) String() string {
	builder := stringify.NewStructBuilder("Commitment",
		stringify.NewStructField("ID", c.id),
		stringify.NewStructField("Commitment", c.commitment),
		stringify.NewStructField("Solid", c.solid),
		stringify.NewStructField("Chain", c.chain),
		stringify.NewStructField("MainChildID", c.mainChildID),
		stringify.NewStructField("MainChildID", c.mainChildID),
	)

	for index, child := range c.children.AsMap() {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("children%d", index), child.ID()))
	}

	return builder.String()
}
