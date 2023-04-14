package booker

import (
	"sync"

	"github.com/iotaledger/hive.go/crypto/identity"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
)

type Block struct {
	booked    bool
	mutex     sync.RWMutex
	witnesses *advancedset.AdvancedSet[identity.ID]

	*BlockDAGBlock
}

type BlockDAGBlock = blockdag.Block

func NewBlock(block *blockdag.Block) *Block {
	return &Block{
		witnesses: advancedset.New[identity.ID](),
		BlockDAGBlock:     block,
	}
}

func NewRootBlock(block *blockdag.Block) *Block {
	return &Block{
		witnesses: advancedset.New[identity.ID](),
		BlockDAGBlock:     block,
		booked:    true,
	}
}

func (b *Block) IsBooked() (isBooked bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.booked
}

func (b *Block) SetBooked() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.booked; wasUpdated {
		b.booked = true
	}

	return
}

func (b *Block) AddWitness(id identity.ID) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.witnesses.Add(id)
}

func (b *Block) String() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	builder := stringify.NewStructBuilder("Booker.Block", stringify.NewStructField("id", b.ID()))
	builder.AddField(stringify.NewStructField("Booked", b.booked))
	builder.AddField(stringify.NewStructField("Witnesses", b.witnesses))

	return builder.String()
}

// Blocks represents a collection of Block.
type Blocks = *advancedset.AdvancedSet[*Block]

// NewBlocks returns a new Block collection with the given elements.
func NewBlocks(blocks ...*Block) (newBlocks Blocks) {
	return advancedset.New(blocks...)
}
