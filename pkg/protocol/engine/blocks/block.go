package blocks

import (
	"fmt"
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/core/reactive"
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
	witnesses *advancedset.AdvancedSet[account.SeatIndex]
	// conflictIDs are the all conflictIDs of the block inherited from the parents + payloadConflictIDs.
	conflictIDs *advancedset.AdvancedSet[iotago.TransactionID]
	// payloadConflictIDs are the conflictIDs of the block's payload (in case it is a transaction, otherwise empty).
	payloadConflictIDs *advancedset.AdvancedSet[iotago.TransactionID]

	// BlockGadget block
	preAccepted           bool
	acceptanceRatifiers   *advancedset.AdvancedSet[account.SeatIndex]
	accepted              reactive.Variable[bool]
	preConfirmed          bool
	confirmationRatifiers *advancedset.AdvancedSet[account.SeatIndex]
	confirmed             bool

	mutex sync.RWMutex

	modelBlock *model.Block
	rootBlock  *rootBlock
}

type rootBlock struct {
	blockID      iotago.BlockID
	commitmentID iotago.CommitmentID
	issuingTime  time.Time
}

func (r *rootBlock) String() string {
	builder := stringify.NewStructBuilder("rootBlock")
	builder.AddField(stringify.NewStructField("blockID", r.blockID))
	builder.AddField(stringify.NewStructField("commitmentID", r.commitmentID))
	builder.AddField(stringify.NewStructField("issuingTime", r.issuingTime))

	return builder.String()
}

// NewBlock creates a new Block with the given options.
func NewBlock(data *model.Block) *Block {
	return &Block{
		witnesses:             advancedset.New[account.SeatIndex](),
		conflictIDs:           advancedset.New[iotago.TransactionID](),
		payloadConflictIDs:    advancedset.New[iotago.TransactionID](),
		acceptanceRatifiers:   advancedset.New[account.SeatIndex](),
		confirmationRatifiers: advancedset.New[account.SeatIndex](),
		modelBlock:            data,
		accepted:              reactive.NewVariable[bool](),
	}
}

func NewRootBlock(blockID iotago.BlockID, commitmentID iotago.CommitmentID, issuingTime time.Time) *Block {
	b := &Block{
		witnesses:             advancedset.New[account.SeatIndex](),
		conflictIDs:           advancedset.New[iotago.TransactionID](),
		payloadConflictIDs:    advancedset.New[iotago.TransactionID](),
		acceptanceRatifiers:   advancedset.New[account.SeatIndex](),
		confirmationRatifiers: advancedset.New[account.SeatIndex](),

		rootBlock: &rootBlock{
			blockID:      blockID,
			commitmentID: commitmentID,
			issuingTime:  issuingTime,
		},
		solid:       true,
		booked:      true,
		preAccepted: true,
		accepted:    reactive.NewVariable[bool](),
	}

	// This should be true since we commit and evict on acceptance.
	b.accepted.Set(true)

	return b
}

func NewMissingBlock(blockID iotago.BlockID) *Block {
	return &Block{
		missing:               true,
		missingBlockID:        blockID,
		witnesses:             advancedset.New[account.SeatIndex](),
		conflictIDs:           advancedset.New[iotago.TransactionID](),
		payloadConflictIDs:    advancedset.New[iotago.TransactionID](),
		acceptanceRatifiers:   advancedset.New[account.SeatIndex](),
		confirmationRatifiers: advancedset.New[account.SeatIndex](),
		accepted:              reactive.NewVariable[bool](),
	}
}

func (b *Block) ProtocolBlock() *iotago.ProtocolBlock {
	if b.modelBlock == nil {
		return nil
	}

	return b.modelBlock.ProtocolBlock()
}

func (b *Block) Parents() (parents []iotago.BlockID) {
	return b.modelBlock.ProtocolBlock().Parents()
}

func (b *Block) StrongParents() (parents []iotago.BlockID) {
	return b.modelBlock.ProtocolBlock().Block.StrongParentIDs()
}

// ParentsWithType returns the parents of the block with their type.
func (b *Block) ParentsWithType() []iotago.Parent {
	return b.modelBlock.ProtocolBlock().ParentsWithType()
}

// ForEachParent executes a consumer func for each parent.
func (b *Block) ForEachParent(consumer func(parent iotago.Parent)) {
	b.modelBlock.ProtocolBlock().ForEachParent(consumer)
}

func (b *Block) IsRootBlock() bool {
	return b.rootBlock != nil
}

func (b *Block) Transaction() (tx *iotago.Transaction, hasTransaction bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.Transaction()
}

func (b *Block) BasicBlock() (basicBlock *iotago.BasicBlock, isBasicBlock bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.BasicBlock()
}

func (b *Block) ValidatorBlock() (validatorBlock *iotago.ValidatorBlock, isValidatorBlock bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.ValidatorBlock()
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

	return b.modelBlock.ProtocolBlock().IssuingTime
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

	return b.modelBlock.ProtocolBlock().SlotCommitmentID
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

func (b *Block) AppendChild(child *Block, childType iotago.ParentsType) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	switch childType {
	case iotago.StrongParentType:
		b.strongChildren = append(b.strongChildren, child)
	case iotago.WeakParentType:
		b.weakChildren = append(b.weakChildren, child)
	case iotago.ShallowLikeParentType:
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

func (b *Block) AddWitness(seat account.SeatIndex) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.witnesses.Add(seat)
}

func (b *Block) Witnesses() []account.SeatIndex {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.witnesses.Slice()
}

func (b *Block) ConflictIDs() *advancedset.AdvancedSet[iotago.TransactionID] {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.conflictIDs
}

func (b *Block) SetConflictIDs(conflictIDs *advancedset.AdvancedSet[iotago.TransactionID]) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.conflictIDs = conflictIDs
}

func (b *Block) PayloadConflictIDs() *advancedset.AdvancedSet[iotago.TransactionID] {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.payloadConflictIDs
}

func (b *Block) SetPayloadConflictIDs(payloadConflictIDs *advancedset.AdvancedSet[iotago.TransactionID]) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.payloadConflictIDs = payloadConflictIDs
}

// IsPreAccepted returns true if the Block was preAccepted.
func (b *Block) IsPreAccepted() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.preAccepted
}

// SetPreAccepted sets the Block as preAccepted.
func (b *Block) SetPreAccepted() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.preAccepted; wasUpdated {
		b.preAccepted = true
	}

	return wasUpdated
}

func (b *Block) AddAcceptanceRatifier(seat account.SeatIndex) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.acceptanceRatifiers.Add(seat)
}

func (b *Block) AcceptanceRatifiers() []account.SeatIndex {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.acceptanceRatifiers.Slice()
}

// IsAccepted returns true if the Block was accepted.
func (b *Block) IsAccepted() bool {
	return b.accepted.Get()
}

// SetAccepted sets the Block as accepted.
func (b *Block) SetAccepted() (wasUpdated bool) {
	return !b.accepted.Set(true)
}

// Accepted returns a reactive variable that is true if the Block was accepted.
func (b *Block) Accepted() reactive.Variable[bool] {
	return b.accepted
}

func (b *Block) AddConfirmationRatifier(seat account.SeatIndex) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.confirmationRatifiers.Add(seat)
}

func (b *Block) ConfirmationRatifiers() []account.SeatIndex {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.confirmationRatifiers.Slice()
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

func (b *Block) IsPreConfirmed() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.preConfirmed
}

func (b *Block) SetPreConfirmed() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.preConfirmed; wasUpdated {
		b.preConfirmed = true
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
	builder.AddField(stringify.NewStructField("PreAccepted", b.preAccepted))
	builder.AddField(stringify.NewStructField("AcceptanceRatifiers", b.acceptanceRatifiers))
	builder.AddField(stringify.NewStructField("Accepted", b.accepted))
	builder.AddField(stringify.NewStructField("PreConfirmed", b.preConfirmed))
	builder.AddField(stringify.NewStructField("ConfirmationRatifiers", b.confirmationRatifiers))
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
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.modelBlock
}
