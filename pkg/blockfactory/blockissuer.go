package blockfactory

import (
	"context"
	"crypto/ed25519"
	"time"

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
	ErrBlockAttacherInvalidBlock              = ierrors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible      = ierrors.New("attaching not possible")
	ErrBlockAttacherIncompleteBlockNotAllowed = ierrors.New("incomplete block is not allowed on this node")
)

// TODO: make sure an honest validator does not issue blocks within the same slot ratification period in two conflicting chains.
//  - this can be achieved by remembering the last issued block together with the engine name/chain.
//  - if the engine name/chain is the same we can always issue a block.
//  - if the engine name/chain is different we need to make sure to wait "slot ratification" slots.

// BlockIssuer contains logic to create and issue blocks signed by the given account.
type BlockIssuer struct {
	events *Events

	workerPool *workerpool.WorkerPool

	Account  Account
	protocol *protocol.Protocol

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
	// optsIncompleteBlockAccepted defines whether the node allows filling in incomplete block and issuing it for user.
	optsIncompleteBlockAccepted bool
	optsRateSetterEnabled       bool
}

func New(p *protocol.Protocol, account Account, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		events:                      NewEvents(),
		workerPool:                  p.Workers.CreatePool("BlockIssuer"),
		protocol:                    p,
		Account:                     account,
		optsIncompleteBlockAccepted: false,
		optsRateSetterEnabled:       false,
		//optsTipSelectionTimeout:       5 * time.Second, // TODO: what should the default be?
		//optsTipSelectionRetryInterval: 200 * time.Millisecond, // TODO: what should the default be?
	}, opts)
}

// Shutdown shuts down the block issuer.
func (i *BlockIssuer) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

func (i *BlockIssuer) CreateValidationBlock(ctx context.Context, opts ...options.Option[BlockParams]) (*model.Block, error) {
	blockParams := options.Apply(&BlockParams{}, opts)

	if blockParams.LatestFinalizedSlot == nil {
		latestFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		blockParams.LatestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.IssuingTime = &issuingTime
	}

	if blockParams.SlotCommitment == nil {
		selectedCommitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		blockIndex := i.protocol.CurrentAPI().TimeProvider().SlotFromTime(*blockParams.IssuingTime)
		minCommittableAge := i.protocol.CurrentAPI().ProtocolParameters().MinCommittableAge()
		maxCommittableAge := i.protocol.CurrentAPI().ProtocolParameters().MaxCommittableAge()

		// If the latest commitment is either too recent or too old for the given issuing time,
		// then use the latest possible commitment.
		// The "+1" element is there because we're comparing against 'blockIndex' which is in the middle of a slot
		// and the oldest possible committed slot is 'minCommittableAge' full slots in the past.
		// So we need to subtract 1 to account for the blockIndex slot that is not finished yet.
		if blockIndex < selectedCommitment.Index+minCommittableAge+1 {
			validCommitmentIndex := iotago.SlotIndex(0)
			if blockIndex > minCommittableAge+1 {
				validCommitmentIndex = blockIndex - 1 - minCommittableAge
			}

			commitment, err := i.protocol.MainEngineInstance().Storage.Commitments().Load(validCommitmentIndex)
			if err != nil {
				return nil, ierrors.Wrapf(err, "error loading commitment for slot %d - latest committed one %d is too recent for the issuing time %s", validCommitmentIndex, selectedCommitment, blockParams.IssuingTime.String())
			}

			selectedCommitment = commitment.Commitment()
		}

		if blockIndex > selectedCommitment.Index+maxCommittableAge+1 {
			return nil, ierrors.Errorf("error building block: do not have valid commitment for issuing time %s - latest available commitment: %d, latest valid commitment: %d", blockParams.IssuingTime.String(), selectedCommitment.Index, blockIndex-1-maxCommittableAge)
		}

		blockParams.SlotCommitment = selectedCommitment
	}

	if blockParams.References == nil {
		references, err := i.getReferences(ctx, blockParams.Payload, blockParams.ParentsCount)
		if err != nil {
			return nil, ierrors.Wrap(err, "error building block")
		}
		blockParams.References = references
	}

	if blockParams.Issuer == nil {
		blockParams.Issuer = NewEd25519Account(i.Account.ID(), i.Account.PrivateKey())
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

	if err := i.validateReferences(*blockParams.IssuingTime, blockParams.SlotCommitment.Index, blockParams.References); err != nil {
		return nil, ierrors.Wrap(err, "block references invalid")
	}

	var api iotago.API
	if blockParams.ProtocolVersion != nil {
		var err error
		api, err = i.protocol.APIForVersion(*blockParams.ProtocolVersion)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error getting api for version %d", *blockParams.ProtocolVersion)
		}
	} else {
		api = i.protocol.CurrentAPI()
	}

	blockBuilder := builder.NewValidationBlockBuilder(api)
	blockBuilder.SlotCommitmentID(blockParams.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.IssuingTime)
	blockBuilder.HighestSupportedVersion(*blockParams.HighestSupportedVersion)
	blockBuilder.ProtocolParametersHash(*blockParams.ProtocolParametersHash)

	if strongParents, exists := blockParams.References[iotago.StrongParentType]; exists && len(strongParents) > 0 {
		blockBuilder.StrongParents(strongParents)
	} else {
		return nil, ierrors.New("cannot create a block without strong parents")
	}

	if weakParents, exists := blockParams.References[iotago.WeakParentType]; exists {
		blockBuilder.WeakParents(weakParents)
	}

	if shallowLikeParents, exists := blockParams.References[iotago.ShallowLikeParentType]; exists {
		blockBuilder.ShallowLikeParents(shallowLikeParents)
	}

	blockBuilder.Sign(i.Account.ID(), i.Account.PrivateKey())

	block, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "error building block")
	}

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, api, serix.WithValidation())
	if err != nil {
		return nil, ierrors.Wrap(err, "error serializing block to model block")
	}

	i.events.BlockConstructed.Trigger(modelBlock)

	return modelBlock, nil
}

// CreateBlock creates a new block with the options.
func (i *BlockIssuer) CreateBlock(ctx context.Context, opts ...options.Option[BlockParams]) (*model.Block, error) {
	blockParams := options.Apply(&BlockParams{}, opts)

	if blockParams.LatestFinalizedSlot == nil {
		latestFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		blockParams.LatestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.IssuingTime = &issuingTime
	}

	if blockParams.SlotCommitment == nil {
		selectedCommitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		blockIndex := i.protocol.CurrentAPI().TimeProvider().SlotFromTime(*blockParams.IssuingTime)
		minCommittableAge := i.protocol.CurrentAPI().ProtocolParameters().MinCommittableAge()
		maxCommittableAge := i.protocol.CurrentAPI().ProtocolParameters().MaxCommittableAge()

		// If the latest commitment is either too recent or too old for the given issuing time,
		// then use the latest possible commitment.
		// The "+1" element is there because we're comparing against 'blockIndex' which is in the middle of a slot
		// and the oldest possible committed slot is 'minCommittableAge' full slots in the past.
		// So we need to subtract 1 to account for the blockIndex slot that is not finished yet.
		if blockIndex < selectedCommitment.Index+minCommittableAge+1 {
			validCommitmentIndex := iotago.SlotIndex(0)
			if blockIndex > minCommittableAge+1 {
				validCommitmentIndex = blockIndex - 1 - minCommittableAge
			}

			commitment, err := i.protocol.MainEngineInstance().Storage.Commitments().Load(validCommitmentIndex)
			if err != nil {
				return nil, ierrors.Wrapf(err, "error loading commitment for slot %d - latest committed one %d is too recent for the issuing time %s", validCommitmentIndex, selectedCommitment, blockParams.IssuingTime.String())
			}

			selectedCommitment = commitment.Commitment()
		}

		if blockIndex > selectedCommitment.Index+maxCommittableAge+1 {
			return nil, ierrors.Errorf("error building block: do not have valid commitment for issuing time %s - latest available commitment: %d, latest valid commitment: %d", blockParams.IssuingTime.String(), selectedCommitment.Index, blockIndex-1-maxCommittableAge)
		}

		blockParams.SlotCommitment = selectedCommitment
	}

	if blockParams.References == nil {
		references, err := i.getReferences(ctx, blockParams.Payload, blockParams.ParentsCount)
		if err != nil {
			return nil, ierrors.Wrap(err, "error building block")
		}
		blockParams.References = references
	}

	if blockParams.Issuer == nil {
		blockParams.Issuer = NewEd25519Account(i.Account.ID(), i.Account.PrivateKey())
	}

	if err := i.validateReferences(*blockParams.IssuingTime, blockParams.SlotCommitment.Index, blockParams.References); err != nil {
		return nil, ierrors.Wrap(err, "block references invalid")
	}

	var api iotago.API
	if blockParams.ProtocolVersion != nil {
		var err error
		api, err = i.protocol.APIForVersion(*blockParams.ProtocolVersion)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error getting api for version %d", *blockParams.ProtocolVersion)
		}
	} else {
		api = i.protocol.CurrentAPI()
	}

	blockBuilder := builder.NewBasicBlockBuilder(api)
	blockBuilder.Payload(blockParams.Payload)
	blockBuilder.SlotCommitmentID(blockParams.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.IssuingTime)

	if strongParents, exists := blockParams.References[iotago.StrongParentType]; exists && len(strongParents) > 0 {
		blockBuilder.StrongParents(strongParents)
	} else {
		return nil, ierrors.New("cannot create a block without strong parents")
	}

	if weakParents, exists := blockParams.References[iotago.WeakParentType]; exists {
		blockBuilder.WeakParents(weakParents)
	}

	if shallowLikeParents, exists := blockParams.References[iotago.ShallowLikeParentType]; exists {
		blockBuilder.ShallowLikeParents(shallowLikeParents)
	}

	blockBuilder.Sign(i.Account.ID(), i.Account.PrivateKey())

	block, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Wrap(err, "error building block")
	}

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, api, serix.WithValidation())
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

func (i *BlockIssuer) AttachBlock(ctx context.Context, iotaBlock *iotago.ProtocolBlock) (iotago.BlockID, error) {
	// if anything changes, need to make a new signature
	var resign bool

	apiForVesion, err := i.protocol.APIForVersion(iotaBlock.ProtocolVersion)
	if err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "protocolVersion invalid: %d", iotaBlock.ProtocolVersion)
	}

	protoParams := apiForVesion.ProtocolParameters()

	if iotaBlock.NetworkID == 0 {
		iotaBlock.NetworkID = protoParams.NetworkID()
		resign = true
	}

	if iotaBlock.SlotCommitmentID == iotago.EmptyCommitmentID {
		iotaBlock.SlotCommitmentID = i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID()
		iotaBlock.LatestFinalizedSlot = i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		resign = true
	}

	switch innerBlock := iotaBlock.Block.(type) {
	case *iotago.BasicBlock:
		switch payload := innerBlock.Payload.(type) {
		case *iotago.Transaction:
			if payload.Essence.NetworkID != protoParams.NetworkID() {
				return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "invalid payload, error: wrong networkID: %d", payload.Essence.NetworkID)
			}
		}

		if len(iotaBlock.Parents()) == 0 {
			references, referencesErr := i.getReferences(ctx, innerBlock.Payload)
			if referencesErr != nil {
				return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherAttachingNotPossible, "tipselection failed, error: %w", referencesErr)
			}

			innerBlock.StrongParents = references[iotago.StrongParentType]
			innerBlock.WeakParents = references[iotago.WeakParentType]
			innerBlock.ShallowLikeParents = references[iotago.ShallowLikeParentType]
			resign = true
		}

	case *iotago.ValidationBlock:
		//nolint:revive,staticcheck //temporarily disable
		if len(iotaBlock.Parents()) == 0 {
			// TODO: implement tipselection for validator blocks
		}
	}

	references := make(model.ParentReferences)
	references[iotago.StrongParentType] = iotaBlock.Block.StrongParentIDs().RemoveDupsAndSort()
	references[iotago.WeakParentType] = iotaBlock.Block.WeakParentIDs().RemoveDupsAndSort()
	references[iotago.ShallowLikeParentType] = iotaBlock.Block.ShallowLikeParentIDs().RemoveDupsAndSort()
	if iotaBlock.IssuingTime.Equal(time.Unix(0, 0)) {
		iotaBlock.IssuingTime = time.Now().UTC()
		resign = true
	}

	if err = i.validateReferences(iotaBlock.IssuingTime, iotaBlock.SlotCommitmentID.Index(), references); err != nil {
		return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherAttachingNotPossible, "invalid block references, error: %w", err)
	}

	if iotaBlock.IssuerID.Empty() || resign {
		if i.optsIncompleteBlockAccepted {
			iotaBlock.IssuerID = i.Account.ID()

			prvKey := i.Account.PrivateKey()
			signature, signatureErr := iotaBlock.Sign(apiForVesion, iotago.NewAddressKeysForEd25519Address(iotago.Ed25519AddressFromPubKey(prvKey.Public().(ed25519.PublicKey)), prvKey))
			if signatureErr != nil {
				return iotago.EmptyBlockID(), ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "%w", signatureErr)
			}

			edSig, isEdSig := signature.(*iotago.Ed25519Signature)
			if !isEdSig {
				return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherInvalidBlock, "unsupported signature type")
			}

			iotaBlock.Signature = edSig
		} else {
			return iotago.EmptyBlockID(), ierrors.Wrap(ErrBlockAttacherIncompleteBlockNotAllowed, "signature needed")
		}
	}

	modelBlock, err := model.BlockFromBlock(iotaBlock, apiForVesion)
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
		if b.SlotCommitmentID().Index() > slotCommitmentIndex {
			return ierrors.Errorf("cannot issue block if the commitment is ahead of its parents' commitment: %s vs %s", b.SlotCommitmentID().Index(), slotCommitmentIndex)
		}
	}

	return nil
}

func (i *BlockIssuer) issueBlock(block *model.Block) error {
	if err := i.protocol.ProcessOwnBlock(block); err != nil {
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

func WithIncompleteBlockAccepted(accepted bool) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsIncompleteBlockAccepted = accepted
	}
}

func WithRateSetterEnabled(enabled bool) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsRateSetterEnabled = enabled
	}
}
