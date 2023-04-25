package blocks

import (
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	// BlockDAG block
	missing             bool
	missingBlockID      iotago.BlockID
	solid               bool
	invalid             bool
	future              bool
	strongChildren      []*Block
	weakChildren        []*Block
	shallowLikeChildren []*Block

	// Booker block
	booked    bool
	witnesses *advancedset.AdvancedSet[iotago.AccountID]

	// BlockGadget block
	accepted         bool
	ratifiers        *advancedset.AdvancedSet[iotago.AccountID]
	ratifiedAccepted bool
	confirmed        bool

	mutex sync.RWMutex

	modelBlock *model.Block
	rootBlock  *rootBlock
}

type rootBlock struct {
	blockID      iotago.BlockID
	commitmentID iotago.CommitmentID
	issuingTime  time.Time
}

// NewBlock creates a new Block with the given options.
func NewBlock(data *model.Block) *Block {
	return &Block{
		witnesses:  advancedset.New[iotago.AccountID](),
		ratifiers:  advancedset.New[iotago.AccountID](),
		modelBlock: data,
	}
}

func NewRootBlock(blockID iotago.BlockID, commitmentID iotago.CommitmentID, issuingTime time.Time) *Block {
	return &Block{
		witnesses: advancedset.New[iotago.AccountID](),
		ratifiers: advancedset.New[iotago.AccountID](),
		rootBlock: &rootBlock{
			blockID:      blockID,
			commitmentID: commitmentID,
			issuingTime:  issuingTime,
		},
		solid:            true,
		booked:           true,
		accepted:         true,
		ratifiedAccepted: true, // TODO: check if this should be true
		confirmed:        true, // TODO: check if this should be true
	}
}

func NewMissingBlock(blockID iotago.BlockID) *Block {
	return &Block{
		missing:        true,
		missingBlockID: blockID,
		witnesses:      advancedset.New[iotago.AccountID](),
		ratifiers:      advancedset.New[iotago.AccountID](),
	}
}

func (b *Block) Block() *iotago.Block {
	return b.modelBlock.Block()
}

// TODO: maybe move to iota.go and introduce parent type.
func (b *Block) Parents() (parents []iotago.BlockID) {
	return b.modelBlock.Parents()
}

func (b *Block) StrongParents() (parents []iotago.BlockID) {
	return b.modelBlock.Block().StrongParents
}

// ForEachParent executes a consumer func for each parent.
func (b *Block) ForEachParent(consumer func(parent model.Parent)) {
	b.modelBlock.ForEachParent(consumer)
}

func (b *Block) IsRootBlock() bool {
	return b.rootBlock != nil
}

func (b *Block) ID() iotago.BlockID {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.missing {
		return b.missingBlockID
	}

	if b.rootBlock != nil {
		return b.rootBlock.blockID
	}

	return b.modelBlock.ID()
}

func (b *Block) IssuingTime() time.Time {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.missing {
		return time.Time{}
	}

	if b.rootBlock != nil {
		return b.rootBlock.issuingTime
	}

	return b.modelBlock.Block().IssuingTime
}

func (b *Block) SlotCommitmentID() iotago.CommitmentID {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if b.missing {
		return iotago.CommitmentID{}
	}

	if b.rootBlock != nil {
		return b.rootBlock.commitmentID
	}

	return b.modelBlock.Block().SlotCommitment.MustID()
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

	b.modelBlock = data
	b.missing = false

	return true
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

func (b *Block) AddWitness(id iotago.AccountID) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.witnesses.Add(id)
}

func (b *Block) Witnesses() []iotago.AccountID {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.witnesses.Slice()
}

// IsAccepted returns true if the Block was accepted.
func (b *Block) IsAccepted() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.accepted
}

// SetAccepted sets the Block as accepted.
func (b *Block) SetAccepted() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.accepted; wasUpdated {
		b.accepted = true
	}

	return wasUpdated
}

func (b *Block) AddRatifier(id iotago.AccountID) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.ratifiers.Add(id)
}

func (b *Block) Ratifiers() []iotago.AccountID {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.ratifiers.Slice()
}

// IsRatifiedAccepted returns true if the Block was ratified accepted.
func (b *Block) IsRatifiedAccepted() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.ratifiedAccepted
}

// SetRatifiedAccepted sets the Block as ratified accepted.
func (b *Block) SetRatifiedAccepted() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.ratifiedAccepted; wasUpdated {
		b.ratifiedAccepted = true
	}

	return wasUpdated
}

func (b *Block) IsConfirmed() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.confirmed
}

func (b *Block) SetConfirmed() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.confirmed; wasUpdated {
		b.confirmed = true
	}

	return wasUpdated
}

func (b *Block) String() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	builder := stringify.NewStructBuilder("Engine.Block", stringify.NewStructField("id", b.ID()))
	builder.AddField(stringify.NewStructField("Missing", b.missing))
	builder.AddField(stringify.NewStructField("Solid", b.solid))
	builder.AddField(stringify.NewStructField("Invalid", b.invalid))
	builder.AddField(stringify.NewStructField("Future", b.future))
	builder.AddField(stringify.NewStructField("Booked", b.booked))
	builder.AddField(stringify.NewStructField("Witnesses", b.witnesses))
	builder.AddField(stringify.NewStructField("Accepted", b.accepted))
	builder.AddField(stringify.NewStructField("Confirmed", b.confirmed))

	for index, child := range b.strongChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("strongChildren%d", index), child.ID().String()))
	}

	for index, child := range b.weakChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("weakChildren%d", index), child.ID().String()))
	}

	for index, child := range b.shallowLikeChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("shallowLikeChildren%d", index), child.ID().String()))
	}

	builder.AddField(stringify.NewStructField("RootBlock", b.rootBlock))
	builder.AddField(stringify.NewStructField("ModelsBlock", b.modelBlock))

	return builder.String()
}

func (b *Block) ModelBlock() *model.Block {
	return b.modelBlock
}
