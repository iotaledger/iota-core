package blockfactory

import (
	"context"
	"time"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

var (
	ErrBlockAttacherInvalidBlock         = ierrors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible = ierrors.New("attaching not possible")
)

// TODO: make sure an honest validator does not issue blocks within the same slot ratification period in two conflicting chains.
//  - this can be achieved by remembering the last issued block together with the engine name/chain.
//  - if the engine name/chain is the same we can always issue a block.
//  - if the engine name/chain is different we need to make sure to wait "slot ratification" slots.

// BlockIssuer contains logic to create and issue blocks signed by the given account.
type BlockIssuer struct {
	events *Events

	workerPool *workerpool.WorkerPool

	protocol *protocol.Protocol

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
	// optsIncompleteBlockAccepted defines whether the node allows filling in incomplete block and issuing it for user.
	optsRateSetterEnabled bool
}

func New(p *protocol.Protocol, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		events:                        NewEvents(),
		workerPool:                    p.Workers.CreatePool("BlockIssuer"),
		protocol:                      p,
		optsRateSetterEnabled:         false,
		optsTipSelectionTimeout:       5 * time.Second,
		optsTipSelectionRetryInterval: 200 * time.Millisecond,
	}, opts)
}

// Shutdown shuts down the block issuer.
func (i *BlockIssuer) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

func (i *BlockIssuer) CreateValidationBlock(ctx context.Context, issuerAccount Account, opts ...options.Option[ValidatorBlockParams]) (*model.Block, error) {
	blockParams := options.Apply(&ValidatorBlockParams{}, opts)

	if blockParams.BlockHeader.References == nil {
		// TODO: change this to get references for validator block
		references, err := i.getReferences(ctx, nil, blockParams.BlockHeader.ParentsCount)
		if err != nil {
			return nil, ierrors.Wrap(err, "error building block")
		}
		blockParams.BlockHeader.References = references
	}

	if err := i.setDefaultBlockParams(blockParams.BlockHeader, issuerAccount); err != nil {
		return nil, err
	}

	if blockParams.HighestSupportedVersion == nil {
		// We use the latest supported version and not the current one.
		version := i.protocol.LatestAPI().Version()
		blockParams.HighestSupportedVersion = &version
	}

	if blockParams.ProtocolParametersHash == nil {
		protocolParametersHash, err := i.protocol.CurrentAPI().ProtocolParameters().Hash()
		if err != nil {
			return nil, ierrors.Wrap(err, "error getting protocol parameters hash")
		}
		blockParams.ProtocolParametersHash = &protocolParametersHash
	}

	api, err := i.retrieveAPI(blockParams.BlockHeader)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error getting api for version %d", *blockParams.BlockHeader.ProtocolVersion)
	}

	blockBuilder := builder.NewValidationBlockBuilder(api)

	blockBuilder.SlotCommitmentID(blockParams.BlockHeader.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.BlockHeader.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.BlockHeader.IssuingTime)

	if strongParents, exists := blockParams.BlockHeader.References[iotago.StrongParentType]; exists && len(strongParents) > 0 {
		blockBuilder.StrongParents(strongParents)
	} else {
		return nil, ierrors.New("cannot create a block without strong parents")
	}

	if weakParents, exists := blockParams.BlockHeader.References[iotago.WeakParentType]; exists {
		blockBuilder.WeakParents(weakParents)
	}

	if shallowLikeParents, exists := blockParams.BlockHeader.References[iotago.ShallowLikeParentType]; exists {
		blockBuilder.ShallowLikeParents(shallowLikeParents)
	}

	blockBuilder.HighestSupportedVersion(*blockParams.HighestSupportedVersion)
	blockBuilder.ProtocolParametersHash(*blockParams.ProtocolParametersHash)

	blockBuilder.Sign(issuerAccount.ID(), issuerAccount.PrivateKey())

	block, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "error building block")
	}

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	if err != nil {
		return nil, ierrors.Wrap(err, "error serializing block to model block")
	}

	i.events.BlockConstructed.Trigger(modelBlock)

	return modelBlock, nil
}

func (i *BlockIssuer) retrieveAPI(blockParams *BlockHeaderParams) (iotago.API, error) {
	if blockParams.ProtocolVersion != nil {
		return i.protocol.APIForVersion(*blockParams.ProtocolVersion)
	}

	return i.protocol.CurrentAPI(), nil
}

// CreateBlock creates a new block with the options.
func (i *BlockIssuer) CreateBlock(ctx context.Context, issuerAccount Account, opts ...options.Option[BasicBlockParams]) (*model.Block, error) {
	blockParams := options.Apply(&BasicBlockParams{}, opts)

	if blockParams.BlockHeader.References == nil {
		references, err := i.getReferences(ctx, blockParams.Payload, blockParams.BlockHeader.ParentsCount)
		if err != nil {
			return nil, ierrors.Wrap(err, "error building block")
		}
		blockParams.BlockHeader.References = references
	}

	if err := i.setDefaultBlockParams(blockParams.BlockHeader, issuerAccount); err != nil {
		return nil, err
	}

	api, err := i.retrieveAPI(blockParams.BlockHeader)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error getting api for version %d", *blockParams.BlockHeader.ProtocolVersion)
	}

	blockBuilder := builder.NewBasicBlockBuilder(api)

	blockBuilder.SlotCommitmentID(blockParams.BlockHeader.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.BlockHeader.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.BlockHeader.IssuingTime)
	if strongParents, exists := blockParams.BlockHeader.References[iotago.StrongParentType]; exists && len(strongParents) > 0 {
		blockBuilder.StrongParents(strongParents)
	} else {
		return nil, ierrors.New("cannot create a block without strong parents")
	}

	if weakParents, exists := blockParams.BlockHeader.References[iotago.WeakParentType]; exists {
		blockBuilder.WeakParents(weakParents)
	}

	if shallowLikeParents, exists := blockParams.BlockHeader.References[iotago.ShallowLikeParentType]; exists {
		blockBuilder.ShallowLikeParents(shallowLikeParents)
	}

	blockBuilder.Payload(blockParams.Payload)

	rmcSlot, err := safemath.SafeSub(api.TimeProvider().SlotFromTime(*blockParams.BlockHeader.IssuingTime), api.ProtocolParameters().MaxCommittableAge())
	if err != nil {
		rmcSlot = 0
	}
	rmc, err := i.protocol.MainEngineInstance().Ledger.RMCManager().RMC(rmcSlot)
	if err != nil {
		return nil, ierrors.Wrapf(err, "error loading commitment of slot %d from storage to get RMC", rmcSlot)
	}

	// only set the burned Mana as the last step before signing, so workscore calculation is correct.
	blockBuilder.MaxBurnedMana(rmc)

	blockBuilder.Sign(issuerAccount.ID(), issuerAccount.PrivateKey())

	block, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "error building block")
	}

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	if err != nil {
		return nil, ierrors.Wrap(err, "error serializing block to model block")
	}

	i.events.BlockConstructed.Trigger(modelBlock)

	return modelBlock, nil
}

// IssueBlock submits a block to be processed.
func (i *BlockIssuer) IssueBlock(block *model.Block) error {
	return i.issueBlock(block)
}

// IssueBlockAndAwaitEvent submits a block to be processed and waits for the event to be triggered.
func (i *BlockIssuer) IssueBlockAndAwaitEvent(ctx context.Context, block *model.Block, evt *event.Event1[*blocks.Block]) error {
	triggered := make(chan error, 1)
	exit := make(chan struct{})
	defer close(exit)

	defer evt.Hook(func(eventBlock *blocks.Block) {
		if block.ID() != eventBlock.ID() {
			return
		}
		select {
		case triggered <- nil:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	defer i.protocol.Events.Engine.Filter.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		if block.ID() != event.Block.ID() {
			return
		}
		select {
		case triggered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.issueBlock(block); err != nil {
		return ierrors.Wrapf(err, "failed to issue block %s", block.ID())
	}

	select {
	case <-ctx.Done():
		return ierrors.Errorf("context canceled whilst waiting for event on block %s", block.ID())
	case err := <-triggered:
		if err != nil {
			return ierrors.Wrapf(err, "block filtered out %s", block.ID())
		}

		return nil
	}
}

func (i *BlockIssuer) getBlockRMC(api iotago.API, protocolBlock *iotago.ProtocolBlock) (iotago.Mana, error) {
	rmcSlot, err := safemath.SafeSub(api.TimeProvider().SlotFromTime(protocolBlock.IssuingTime), api.ProtocolParameters().MaxCommittableAge())
	if err != nil {
		rmcSlot = 0
	}

	rmc, err := i.protocol.MainEngineInstance().Ledger.RMCManager().RMC(rmcSlot)
	if err != nil {
		return 0, ierrors.Wrapf(err, "error loading commitment of slot %d from storage to get RMC", rmcSlot)
	}

	return rmc, nil
}

func (i *BlockIssuer) AttachBlock(ctx context.Context, iotaBlock *iotago.ProtocolBlock) (iotago.BlockID, error) {
	apiForVesion, err := i.protocol.APIForVersion(iotaBlock.ProtocolVersion)
	if err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "protocolVersion invalid: %d", iotaBlock.ProtocolVersion)
	}

	protoParams := apiForVesion.ProtocolParameters()

	if iotaBlock.NetworkID == 0 {
		return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid block, error: missing networkID")
	}

	if iotaBlock.SlotCommitmentID.Empty() {
		return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid block, error: missing slotCommitmentID")
	}

	if iotaBlock.IssuingTime.Equal(time.Unix(0, 0)) {
		return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid block, error: missing issuingTime")
	}

	if iotaBlock.IssuerID.Empty() {
		return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid block, error: signature needed")
	}

	// check the block signature
	valid, err := iotaBlock.VerifySignature()
	if err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "invalid block, error: unable to verify block signature: %w", err)
	}
	if !valid {
		return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid block, error: invalid signature")
	}

	switch innerBlock := iotaBlock.Block.(type) {
	case *iotago.BasicBlock:
		switch payload := innerBlock.Payload.(type) {
		case *iotago.SignedTransaction:
			if payload.Transaction.NetworkID != protoParams.NetworkID() {
				return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "invalid basic block payload, error: wrong networkID: %d", payload.Transaction.NetworkID)
			}
		}

		if len(innerBlock.StrongParentIDs()) == 0 {
			return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid basic block, error: missing strongParents")
		}

	case *iotago.ValidationBlock:
		if len(innerBlock.StrongParentIDs()) == 0 {
			return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "invalid validation block, error: missing strongParents")
		}
	}

	references := make(model.ParentReferences)
	references[iotago.StrongParentType] = iotaBlock.Block.StrongParentIDs().RemoveDupsAndSort()
	references[iotago.WeakParentType] = iotaBlock.Block.WeakParentIDs().RemoveDupsAndSort()
	references[iotago.ShallowLikeParentType] = iotaBlock.Block.ShallowLikeParentIDs().RemoveDupsAndSort()

	if err = i.validateReferences(iotaBlock.IssuingTime, iotaBlock.SlotCommitmentID.Slot(), references); err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherAttachingNotPossible, "invalid block references, error: %w", err)
	}

	// check if the block burns enough mana
	if basicBlock, isBasicBlock := iotaBlock.Block.(*iotago.BasicBlock); isBasicBlock {
		rmc, err := i.getBlockRMC(apiForVesion, iotaBlock)
		if err != nil {
			return iotago.EmptyBlockID(), err
		}

		burnedMana, err := basicBlock.ManaCost(rmc, apiForVesion.ProtocolParameters().WorkScoreStructure())
		if err != nil {
			return iotago.EmptyBlockID(), ierrors.Wrapf(err, "could not calculate Mana cost for block")
		}

		if basicBlock.MaxBurnedMana < burnedMana {
			return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "invalid basic block, error: burnedMana lower than the required minimum to issue the block: %d<%d", basicBlock.MaxBurnedMana, burnedMana)
		}
	}

	modelBlock, err := model.BlockFromBlock(iotaBlock)
	if err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrap(err, "error serializing block to model block")
	}

	if !i.optsRateSetterEnabled || i.protocol.MainEngineInstance().Scheduler.IsBlockIssuerReady(modelBlock.ProtocolBlock().IssuerID) {
		i.events.BlockConstructed.Trigger(modelBlock)

		if err = i.IssueBlockAndAwaitEvent(ctx, modelBlock, i.protocol.Events.Engine.BlockDAG.BlockAttached); err != nil {
			return iotago.EmptyBlockID(), ierrors.Wrap(err, "error issuing model block")
		}
	}

	return modelBlock.ID(), nil
}

func (i *BlockIssuer) setDefaultBlockParams(blockParams *BlockHeaderParams, issuerAccount Account) error {
	if blockParams.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.IssuingTime = &issuingTime
	}

	if blockParams.SlotCommitment == nil {
		var err error
		blockParams.SlotCommitment, err = i.getCommitment(i.protocol.CurrentAPI().TimeProvider().SlotFromTime(*blockParams.IssuingTime))
		if err != nil {
			return ierrors.Wrap(err, "error getting commitment")
		}
	}

	if blockParams.LatestFinalizedSlot == nil {
		latestFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		blockParams.LatestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.Issuer == nil {
		blockParams.Issuer = NewEd25519Account(issuerAccount.ID(), issuerAccount.PrivateKey())
	} else if blockParams.Issuer.ID() != issuerAccount.ID() {
		return ierrors.Errorf("provided issuer account %s, but issuer provided in the block params is different %s", issuerAccount.ID(), blockParams.Issuer.ID())
	}

	if err := i.validateReferences(*blockParams.IssuingTime, blockParams.SlotCommitment.Slot, blockParams.References); err != nil {
		return ierrors.Wrap(err, "block references invalid")
	}

	return nil
}

func (i *BlockIssuer) getCommitment(blockSlot iotago.SlotIndex) (*iotago.Commitment, error) {
	protoParams := i.protocol.CurrentAPI().ProtocolParameters()
	commitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

	if blockSlot > commitment.Slot+protoParams.MaxCommittableAge() {
		return nil, ierrors.Errorf("can't issue block: block slot %d is too far in the future, latest commitment is %d", blockSlot, commitment.Slot)
	}

	if blockSlot < commitment.Slot+protoParams.MinCommittableAge() {
		if blockSlot < protoParams.MinCommittableAge() || commitment.Slot < protoParams.MinCommittableAge() {
			return commitment, nil
		}

		commitmentSlot := commitment.Slot - protoParams.MinCommittableAge()
		loadedCommitment, err := i.protocol.MainEngineInstance().Storage.Commitments().Load(commitmentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error loading valid commitment of slot %d according to minCommittableAge from storage", commitmentSlot)
		}

		return loadedCommitment.Commitment(), nil
	}

	return commitment, nil
}

func (i *BlockIssuer) getReferences(ctx context.Context, p iotago.Payload, strongParentsCountOpt ...int) (model.ParentReferences, error) {
	strongParentsCount := iotago.BlockMaxParents
	if len(strongParentsCountOpt) > 0 && strongParentsCountOpt[0] > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	return i.getReferencesWithRetry(ctx, p, strongParentsCount)
}

func (i *BlockIssuer) validateReferences(issuingTime time.Time, slotCommitmentIndex iotago.SlotIndex, references model.ParentReferences) error {
	for _, parent := range lo.Flatten(lo.Map(lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds })) {
		b, exists := i.protocol.MainEngineInstance().BlockFromCache(parent)
		if !exists {
			return ierrors.Errorf("cannot issue block if the parents are not known: %s", parent)
		}

		if b.IssuingTime().After(issuingTime) {
			return ierrors.Errorf("cannot issue block if the parents issuingTime is ahead block's issuingTime: %s vs %s", b.IssuingTime(), issuingTime)
		}
		if b.SlotCommitmentID().Slot() > slotCommitmentIndex {
			return ierrors.Errorf("cannot issue block if the commitment is ahead of its parents' commitment: %s vs %s", b.SlotCommitmentID().Slot(), slotCommitmentIndex)
		}
	}

	return nil
}

func (i *BlockIssuer) issueBlock(block *model.Block) error {
	if err := i.protocol.IssueBlock(block); err != nil {
		return err
	}

	i.events.BlockIssued.Trigger(block)

	return nil
}

// getReferencesWithRetry tries to get references for the given payload. If it fails, it will retry at regular intervals until
// the timeout is reached.
func (i *BlockIssuer) getReferencesWithRetry(ctx context.Context, _ iotago.Payload, parentsCount int) (references model.ParentReferences, err error) {
	timeout := time.NewTimer(i.optsTipSelectionTimeout)
	interval := time.NewTicker(i.optsTipSelectionRetryInterval)
	defer timeutil.CleanupTimer(timeout)
	defer timeutil.CleanupTicker(interval)

	for {
		references = i.protocol.MainEngineInstance().TipSelection.SelectTips(parentsCount)
		if len(references[iotago.StrongParentType]) > 0 {
			return references, nil
		}

		select {
		case <-interval.C:
			i.events.Error.Trigger(ierrors.Wrap(err, "could not get references"))
			continue
		case <-timeout.C:
			return nil, ierrors.New("timeout while trying to select tips and determine references")
		case <-ctx.Done():
			return nil, ierrors.Errorf("context canceled whilst trying to select tips and determine references: %w", ctx.Err())
		}
	}
}

func WithTipSelectionTimeout(timeout time.Duration) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsTipSelectionTimeout = timeout
	}
}

func WithTipSelectionRetryInterval(interval time.Duration) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsTipSelectionRetryInterval = interval
	}
}

func WithRateSetterEnabled(enabled bool) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsRateSetterEnabled = enabled
	}
}
