package blockdag

import (
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

// region Block ////////////////////////////////////////////////////////////////////////////////////////////////////////

// Block represents a Block annotated with Tangle related metadata.
type Block struct {
	missing             bool
	solid               bool
	invalid             bool
	orphaned            bool
	future              bool
	issuingTime         *time.Time
	blockID             *iotago.BlockID
	strongChildren      []*Block
	weakChildren        []*Block
	shallowLikeChildren []*Block
	mutex               sync.RWMutex

	*ModelsBlock
}

func (b *Block) IssuingTime() time.Time {
	if b.issuingTime != nil {
		return *b.issuingTime
	}

	return b.Block().IssuingTime
}

func (b *Block) ID() iotago.BlockID {
	if b.blockID != nil {
		return *b.blockID
	}

	return b.ModelsBlock.ID()
}

type ModelsBlock = model.Block

// NewBlock creates a new Block with the given options.
func NewBlock(data *model.Block, opts ...options.Option[Block]) (newBlock *Block) {
	return options.Apply(&Block{
		strongChildren:      make([]*Block, 0),
		weakChildren:        make([]*Block, 0),
		shallowLikeChildren: make([]*Block, 0),
		ModelsBlock:         data,
	}, opts)
}

func NewRootBlock(id iotago.BlockID, slotTimeProvider *iotago.SlotTimeProvider, opts ...options.Option[model.Block]) (rootBlock *Block) {
	return NewBlock(
		nil,
		WithBlockID(id),
		WithSolid(true),
		WithMissing(false),
		WithIssuingTime(slotTimeProvider.EndTime(id.Index())),
	)
}

// IsMissing returns a flag that indicates if the underlying Block data hasn't been stored, yet.
func (b *Block) IsMissing() (isMissing bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.missing
}

// IsSolid returns true if the Block is solid (the entire causal history is known).
func (b *Block) IsSolid() (isSolid bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.solid
}

// IsInvalid returns true if the Block was marked as invalid.
func (b *Block) IsInvalid() (isInvalid bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.invalid
}

// IsFuture returns true if the Block is a future Block (we haven't committed to its commitment slot yet).
func (b *Block) IsFuture() (isFuture bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.future
}

// SetFuture marks the Block as future block.
func (b *Block) SetFuture() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.future {
		return false
	}

	b.future = true
	return true
}

// IsOrphaned returns true if the Block is orphaned (either due to being marked as orphaned itself or because it has
// orphaned Blocks in its past cone).
func (b *Block) IsOrphaned() (isOrphaned bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.orphaned
}

// Children returns the children of the Block.
func (b *Block) Children() (children []*Block) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	seenBlockIDs := make(map[iotago.BlockID]types.Empty)
	for _, parentsByType := range [][]*Block{
		b.strongChildren,
		b.weakChildren,
		b.shallowLikeChildren,
	} {
		for _, childMetadata := range parentsByType {
			if _, exists := seenBlockIDs[childMetadata.ID()]; !exists {
				children = append(children, childMetadata)
				seenBlockIDs[childMetadata.ID()] = types.Void
			}
		}
	}

	return children
}

func (b *Block) StrongChildren() []*Block {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return lo.CopySlice(b.strongChildren)
}

func (b *Block) WeakChildren() []*Block {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return lo.CopySlice(b.weakChildren)
}

func (b *Block) ShallowLikeChildren() []*Block {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return lo.CopySlice(b.shallowLikeChildren)
}

// SetSolid marks the Block as solid.
func (b *Block) SetSolid() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.solid; wasUpdated {
		b.solid = true
	}

	return
}

// SetInvalid marks the Block as invalid.
func (b *Block) SetInvalid() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.invalid {
		return false
	}

	b.invalid = true

	return true
}

// SetOrphaned sets the orphaned flag of the Block.
func (b *Block) SetOrphaned(orphaned bool) (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.orphaned == orphaned {
		return false
	}
	b.orphaned = orphaned

	return true
}

func (b *Block) AppendChild(child *Block, childType model.ParentsType) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	switch childType {
	case model.StrongParentType:
		b.strongChildren = append(b.strongChildren, child)
	case model.WeakParentType:
		b.weakChildren = append(b.weakChildren, child)
	case model.ShallowLikeParentType:
		b.shallowLikeChildren = append(b.shallowLikeChildren, child)
	}
}

// Update publishes the given Block data to the underlying Block and marks it as no longer missing.
func (b *Block) Update(data *model.Block) (wasPublished bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.missing {
		return
	}

	b.ModelsBlock = data
	b.missing = false

	return true
}

func (b *Block) String() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	builder := stringify.NewStructBuilder("BlockDAG.Block", stringify.NewStructField("id", b.ID()))
	builder.AddField(stringify.NewStructField("Missing", b.missing))
	builder.AddField(stringify.NewStructField("Solid", b.solid))
	builder.AddField(stringify.NewStructField("Invalid", b.invalid))
	builder.AddField(stringify.NewStructField("Orphaned", b.orphaned))

	for index, child := range b.strongChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("strongChildren%d", index), child.ID().String()))
	}

	for index, child := range b.weakChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("weakChildren%d", index), child.ID().String()))
	}

	for index, child := range b.shallowLikeChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("shallowLikeChildren%d", index), child.ID().String()))
	}

	builder.AddField(stringify.NewStructField("ModelsBlock", b.ModelsBlock))

	return builder.String()
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////

// region Options //////////////////////////////////////////////////////////////////////////////////////////////////////

func WithIssuingTime(issuingTime time.Time) options.Option[Block] {
	return func(block *Block) {
		block.issuingTime = &issuingTime
	}
}

func WithBlockID(blockID iotago.BlockID) options.Option[Block] {
	return func(block *Block) {
		block.blockID = &blockID
	}
}

// WithMissing is a constructor Option for Blocks that initializes the given block with a specific missing flag.
func WithMissing(missing bool) options.Option[Block] {
	return func(block *Block) {
		block.missing = missing
	}
}

// WithSolid is a constructor Option for Blocks that initializes the given block with a specific solid flag.
func WithSolid(solid bool) options.Option[Block] {
	return func(block *Block) {
		block.solid = solid
	}
}

// WithOrphaned is a constructor Option for Blocks that initializes the given block with a specific orphaned flag.
func WithOrphaned(markedOrphaned bool) options.Option[Block] {
	return func(block *Block) {
		block.orphaned = markedOrphaned
	}
}

// endregion ///////////////////////////////////////////////////////////////////////////////////////////////////////////
