package blocks

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/ds/types"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/stringify"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Block struct {
	AllParentsBooked          reactive.Event
	AllDependenciesReady      reactive.Event
	SignedTransactionMetadata reactive.Variable[mempool.SignedTransactionMetadata]

	// BlockDAG block
	missing             bool
	missingBlockID      iotago.BlockID
	solid               reactive.Variable[bool]
	invalid             reactive.Variable[bool]
	strongChildren      []*Block
	weakChildren        []*Block
	shallowLikeChildren []*Block

	// Booker block
	booked    reactive.Variable[bool]
	witnesses ds.Set[account.SeatIndex]
	// spenderIDs are the all spenderIDs of the block inherited from the parents + payloadSpenderIDs.
	spenderIDs ds.Set[iotago.TransactionID]
	// payloadSpenderIDs are the spenderIDs of the block's payload (in case it is a transaction, otherwise empty).
	payloadSpenderIDs ds.Set[iotago.TransactionID]

	// BlockGadget block
	preAccepted           bool
	acceptanceRatifiers   ds.Set[account.SeatIndex]
	accepted              reactive.Variable[bool]
	preConfirmed          bool
	confirmationRatifiers ds.Set[account.SeatIndex]
	confirmed             bool
	weightPropagated      reactive.Variable[bool]

	// Scheduler block
	scheduled bool
	skipped   bool
	enqueued  bool
	dropped   bool

	// Notarization
	notarized reactive.Event

	mutex syncutils.RWMutex

	modelBlock *model.Block
	rootBlock  *rootBlock

	workScore iotago.WorkScore
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
func NewBlock(modelBlock *model.Block) *Block {
	return &Block{
		AllParentsBooked:          reactive.NewEvent(),
		AllDependenciesReady:      reactive.NewEvent(),
		SignedTransactionMetadata: reactive.NewVariable[mempool.SignedTransactionMetadata](),

		witnesses:             ds.NewSet[account.SeatIndex](),
		spenderIDs:            ds.NewSet[iotago.TransactionID](),
		payloadSpenderIDs:     ds.NewSet[iotago.TransactionID](),
		acceptanceRatifiers:   ds.NewSet[account.SeatIndex](),
		confirmationRatifiers: ds.NewSet[account.SeatIndex](),
		modelBlock:            modelBlock,
		solid:                 reactive.NewVariable[bool](),
		invalid:               reactive.NewVariable[bool](),
		booked:                reactive.NewVariable[bool](),
		accepted:              reactive.NewVariable[bool](),
		weightPropagated:      reactive.NewVariable[bool](),
		notarized:             reactive.NewEvent(),
		workScore:             modelBlock.WorkScore(),
	}
}

func NewRootBlock(blockID iotago.BlockID, commitmentID iotago.CommitmentID, issuingTime time.Time) *Block {
	b := &Block{
		AllParentsBooked:          reactive.NewEvent(),
		AllDependenciesReady:      reactive.NewEvent(),
		SignedTransactionMetadata: reactive.NewVariable[mempool.SignedTransactionMetadata](),

		witnesses:             ds.NewSet[account.SeatIndex](),
		spenderIDs:            ds.NewSet[iotago.TransactionID](),
		payloadSpenderIDs:     ds.NewSet[iotago.TransactionID](),
		acceptanceRatifiers:   ds.NewSet[account.SeatIndex](),
		confirmationRatifiers: ds.NewSet[account.SeatIndex](),

		rootBlock: &rootBlock{
			blockID:      blockID,
			commitmentID: commitmentID,
			issuingTime:  issuingTime,
		},
		solid:            reactive.NewVariable[bool](),
		invalid:          reactive.NewVariable[bool](),
		booked:           reactive.NewVariable[bool](),
		preAccepted:      true,
		accepted:         reactive.NewVariable[bool](),
		weightPropagated: reactive.NewVariable[bool](),
		notarized:        reactive.NewEvent(),
		scheduled:        true,
	}

	// This should be true since we commit and evict on acceptance.
	b.AllParentsBooked.Set(true)
	b.AllDependenciesReady.Set(true)
	b.solid.Set(true)
	b.booked.Set(true)
	b.weightPropagated.Set(true)
	b.notarized.Set(true)
	b.accepted.Set(true)

	return b
}

func NewMissingBlock(blockID iotago.BlockID) *Block {
	return &Block{
		AllParentsBooked:          reactive.NewEvent(),
		AllDependenciesReady:      reactive.NewEvent(),
		SignedTransactionMetadata: reactive.NewVariable[mempool.SignedTransactionMetadata](),

		missing:               true,
		missingBlockID:        blockID,
		witnesses:             ds.NewSet[account.SeatIndex](),
		spenderIDs:            ds.NewSet[iotago.TransactionID](),
		payloadSpenderIDs:     ds.NewSet[iotago.TransactionID](),
		acceptanceRatifiers:   ds.NewSet[account.SeatIndex](),
		confirmationRatifiers: ds.NewSet[account.SeatIndex](),
		solid:                 reactive.NewVariable[bool](),
		invalid:               reactive.NewVariable[bool](),
		booked:                reactive.NewVariable[bool](),
		accepted:              reactive.NewVariable[bool](),
		weightPropagated:      reactive.NewVariable[bool](),
		notarized:             reactive.NewEvent(),
	}
}

func (b *Block) ProtocolBlock() *iotago.Block {
	if b.modelBlock == nil {
		return nil
	}

	return b.modelBlock.ProtocolBlock()
}

func (b *Block) Parents() (parents []iotago.BlockID) {
	return b.modelBlock.ProtocolBlock().Parents()
}

func (b *Block) StrongParents() (parents []iotago.BlockID) {
	return b.modelBlock.ProtocolBlock().Body.StrongParentIDs()
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

func (b *Block) Payload() iotago.Payload {
	if b.modelBlock == nil {
		return nil
	}

	return b.modelBlock.Payload()
}

func (b *Block) SignedTransaction() (tx *iotago.SignedTransaction, hasTransaction bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.SignedTransaction()
}

func (b *Block) BasicBlock() (basicBlock *iotago.BasicBlockBody, isBasicBlock bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.BasicBlock()
}

func (b *Block) ValidationBlock() (validationBlock *iotago.ValidationBlockBody, isValidationBlock bool) {
	if b.modelBlock == nil {
		return nil, false
	}

	return b.modelBlock.ValidationBlock()
}

func (b *Block) ID() iotago.BlockID {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.id()
}

func (b *Block) id() iotago.BlockID {
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

	return b.modelBlock.ProtocolBlock().Header.IssuingTime
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

	return b.modelBlock.SlotCommitmentID()
}

// IsMissing returns a flag that indicates if the underlying Block data hasn't been stored, yet.
func (b *Block) IsMissing() (isMissing bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.missing
}

// Solid returns a reactive variable that is true if the Block is solid (the entire causal history is known).
func (b *Block) Solid() (solid reactive.Variable[bool]) {
	return b.solid
}

// IsSolid returns true if the Block is solid (the entire causal history is known).
func (b *Block) IsSolid() (isSolid bool) {
	return b.solid.Get()
}

// SetSolid marks the Block as solid.
func (b *Block) SetSolid() (wasUpdated bool) {
	return !b.solid.Set(true)
}

// Invalid returns a reactive variable that is true if the Block was marked as invalid.
func (b *Block) Invalid() (invalid reactive.Variable[bool]) {
	return b.invalid
}

// IsInvalid returns true if the Block was marked as invalid.
func (b *Block) IsInvalid() (isInvalid bool) {
	return b.invalid.Get()
}

// SetInvalid marks the Block as invalid.
func (b *Block) SetInvalid() (wasUpdated bool) {
	return !b.invalid.Set(true)
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
func (b *Block) Update(modelBlock *model.Block) (wasPublished bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.missing {
		return
	}

	b.modelBlock = modelBlock
	b.workScore = modelBlock.WorkScore()
	b.missing = false

	return true
}

// Booked returns a reactive variable that is true if the Block was booked.
func (b *Block) Booked() reactive.Variable[bool] {
	return b.booked
}

func (b *Block) IsBooked() (isBooked bool) {
	return b.booked.Get()
}

func (b *Block) SetBooked() (wasUpdated bool) {
	return !b.booked.Set(true)
}

func (b *Block) AddWitness(seat account.SeatIndex) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.witnesses.Add(seat)
}

func (b *Block) WitnessCount() int {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.witnesses.Size()
}

func (b *Block) Witnesses() []account.SeatIndex {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.witnesses.ToSlice()
}

func (b *Block) SpenderIDs() ds.Set[iotago.TransactionID] {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.spenderIDs
}

func (b *Block) SetSpenderIDs(spenderIDs ds.Set[iotago.TransactionID]) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.spenderIDs = spenderIDs
}

func (b *Block) PayloadSpenderIDs() ds.Set[iotago.TransactionID] {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.payloadSpenderIDs
}

func (b *Block) SetPayloadSpenderIDs(payloadSpenderIDs ds.Set[iotago.TransactionID]) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.payloadSpenderIDs = payloadSpenderIDs
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

	return b.acceptanceRatifiers.ToSlice()
}

// Accepted returns a reactive variable that is true if the Block was accepted.
func (b *Block) Accepted() reactive.Variable[bool] {
	return b.accepted
}

// IsAccepted returns true if the Block was accepted.
func (b *Block) IsAccepted() bool {
	return b.accepted.Get()
}

// SetAccepted sets the Block as accepted.
func (b *Block) SetAccepted() (wasUpdated bool) {
	return !b.accepted.Set(true)
}

// IsScheduled returns true if the Block was scheduled.
func (b *Block) IsScheduled() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.scheduled
}

// SetScheduled sets the Block as scheduled.
func (b *Block) SetScheduled() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.scheduled; wasUpdated && b.enqueued {
		b.scheduled = true
		b.enqueued = false
	}

	return wasUpdated
}

// IsSkipped returns true if the Block was skipped.
func (b *Block) IsSkipped() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.skipped
}

// SetSkipped sets the Block as skipped.
func (b *Block) SetSkipped() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.skipped; wasUpdated && b.enqueued {
		b.skipped = true
		b.enqueued = false
	}

	return wasUpdated
}

// IsDropped returns true if the Block was dropped.
func (b *Block) IsDropped() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.dropped
}

// SetDropped sets the Block as dropped.
func (b *Block) SetDropped() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.dropped; wasUpdated && b.enqueued {
		b.dropped = true
		b.enqueued = false
	}

	return wasUpdated
}

// IsEnqueued returns true if the Block is currently enqueued in the scheduler.
func (b *Block) IsEnqueued() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.enqueued
}

// SetEnqueued sets the Block as enqueued.
func (b *Block) SetEnqueued() (wasUpdated bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if wasUpdated = !b.enqueued; wasUpdated {
		b.enqueued = true
	}

	return wasUpdated
}

func (b *Block) AddConfirmationRatifier(seat account.SeatIndex) (added bool) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.confirmationRatifiers.Add(seat)
}

func (b *Block) ConfirmationRatifiers() []account.SeatIndex {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.confirmationRatifiers.ToSlice()
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

func (b *Block) WeightPropagated() reactive.Variable[bool] {
	return b.weightPropagated
}

func (b *Block) IsWeightPropagated() bool {
	return b.weightPropagated.Get()
}

func (b *Block) SetWeightPropagated() (wasUpdated bool) {
	return !b.weightPropagated.Set(true)
}

func (b *Block) Notarized() reactive.Event {
	return b.notarized
}

func (b *Block) IsNotarized() (isBooked bool) {
	return b.notarized.Get()
}

func (b *Block) SetNotarized() (wasUpdated bool) {
	return b.notarized.Trigger()
}

func (b *Block) String() string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	builder := stringify.NewStructBuilder("Engine.Block", stringify.NewStructField("id", b.id()))
	builder.AddField(stringify.NewStructField("Missing", b.missing))
	builder.AddField(stringify.NewStructField("Solid", b.solid.Get()))
	builder.AddField(stringify.NewStructField("Invalid", b.invalid.Get()))
	builder.AddField(stringify.NewStructField("Booked", b.booked.Get()))
	builder.AddField(stringify.NewStructField("Witnesses", b.witnesses))
	builder.AddField(stringify.NewStructField("PreAccepted", b.preAccepted))
	builder.AddField(stringify.NewStructField("AcceptanceRatifiers", b.acceptanceRatifiers.String()))
	builder.AddField(stringify.NewStructField("Accepted", b.accepted.Get()))
	builder.AddField(stringify.NewStructField("PreConfirmed", b.preConfirmed))
	builder.AddField(stringify.NewStructField("ConfirmationRatifiers", b.confirmationRatifiers.String()))
	builder.AddField(stringify.NewStructField("Confirmed", b.confirmed))
	builder.AddField(stringify.NewStructField("WeightPropagated", b.weightPropagated.Get()))
	builder.AddField(stringify.NewStructField("Scheduled", b.scheduled))
	builder.AddField(stringify.NewStructField("Dropped", b.dropped))
	builder.AddField(stringify.NewStructField("Skipped", b.skipped))
	builder.AddField(stringify.NewStructField("Enqueued", b.enqueued))
	builder.AddField(stringify.NewStructField("Notarized", b.notarized.Get()))

	for index, child := range b.strongChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("strongChildren%d", index), child.ID().String()))
	}

	for index, child := range b.weakChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("weakChildren%d", index), child.ID().String()))
	}

	for index, child := range b.shallowLikeChildren {
		builder.AddField(stringify.NewStructField(fmt.Sprintf("shallowLikeChildren%d", index), child.ID().String()))
	}

	if b.rootBlock != nil {
		builder.AddField(stringify.NewStructField("RootBlock", b.rootBlock.String()))
	}

	if b.modelBlock != nil {
		builder.AddField(stringify.NewStructField("ModelsBlock", b.modelBlock.String()))
	}

	return builder.String()
}

func (b *Block) ModelBlock() *model.Block {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.modelBlock
}

func (b *Block) WorkScore() iotago.WorkScore {
	return b.workScore
}

func (b *Block) WaitForUTXODependencies(dependencies ds.Set[mempool.StateMetadata]) {
	if dependencies == nil || dependencies.Size() == 0 {
		b.AllDependenciesReady.Trigger()

		return
	}

	var unreferencedOutputCount atomic.Int32
	unreferencedOutputCount.Store(int32(dependencies.Size()))

	dependencies.Range(func(dependency mempool.StateMetadata) {
		dependencyReady := false

		dependency.OnAccepted(func() {
			dependency.OnInclusionSlotUpdated(func(_ iotago.SlotIndex, inclusionSlot iotago.SlotIndex) {
				if !dependencyReady && inclusionSlot <= b.ID().Slot() {
					dependencyReady = true

					if unreferencedOutputCount.Add(-1) == 0 {
						b.AllDependenciesReady.Trigger()
					}
				}
			})
		})
	})
}
