package booker

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blockdag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	booked bool
	mutex  sync.RWMutex

	*blockdag.Block
}

func NewBlock(block *blockdag.Block) *Block {
	return &Block{
		Block: block,
	}
}

func NewRootBlock(id iotago.BlockID, commitmentID iotago.CommitmentID, issuingTime time.Time) *Block {
	return &Block{
		Block:  blockdag.NewRootBlock(id, commitmentID, issuingTime),
		booked: true,
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

func (b *Block) String() string {
	builder := stringify.NewStructBuilder("VirtualVoting.Block", stringify.NewStructField("id", b.ID()))
	builder.AddField(stringify.NewStructField("Booked", b.booked))

	return builder.String()
}

// region Blocks ///////////////////////////////////////////////////////////////////////////////////////////////////////

// Blocks represents a collection of Block.
type Blocks = *advancedset.AdvancedSet[*Block]

// NewBlocks returns a new Block collection with the given elements.
func NewBlocks(blocks ...*Block) (newBlocks Blocks) {
	return advancedset.New(blocks...)
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
