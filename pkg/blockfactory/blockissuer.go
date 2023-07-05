package blockfactory

import (
	"context"
	"crypto/ed25519"
	"time"

	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

var (
	ErrBlockAttacherInvalidBlock              = errors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible      = errors.New("attaching not possible")
	ErrBlockAttacherPoWNotAvailable           = errors.New("proof of work is not available on this node")
	ErrBlockAttacherIncompleteBlockNotAllowed = errors.New("incomplete block is not allowed on this node")
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
	optsPoWEnabled                bool
	optsIncompleteBlockAccepted   bool
}

func New(p *protocol.Protocol, account Account, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	return options.Apply(&BlockIssuer{
		events:     NewEvents(),
		workerPool: p.Workers.CreatePool("BlockIssuer"),
		protocol:   p,
		Account:    account,
	}, opts)
}

// Shutdown shuts down the block issuer.
func (i *BlockIssuer) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

// CreateBlock creates a new block with the options.
func (i *BlockIssuer) CreateBlock(ctx context.Context, opts ...options.Option[BlockParams]) (*model.Block, error) {
	blockParams := options.Apply(&BlockParams{}, opts)

	if blockParams.slotCommitment == nil {
		blockParams.slotCommitment = i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
	}

	if blockParams.latestFinalizedSlot == nil {
		latestFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		blockParams.latestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.issuingTime == nil {
		issuingTime := time.Now()
		blockParams.issuingTime = &issuingTime
	}

	if blockParams.references == nil {
		references, err := i.getReferences(ctx, blockParams.payload, blockParams.parentsCount)
		if err != nil {
			return nil, errors.Wrap(err, "error building block")
		}
		blockParams.references = references
	}

	if blockParams.issuer == nil {
		blockParams.issuer = NewEd25519Account(i.Account.ID(), i.Account.PrivateKey())
	}

	if blockParams.proofOfWorkDifficulty == nil {
		powDifficulty := float64(i.protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().MinPoWScore)
		blockParams.proofOfWorkDifficulty = &powDifficulty
	}

	if err := i.validateReferences(*blockParams.issuingTime, blockParams.slotCommitment.Index, blockParams.references); err != nil {
		return nil, errors.Wrap(err, "block references invalid")
	}

	blockBuilder := builder.NewBlockBuilder()

	if blockParams.protocolVersion != nil {
		blockBuilder.ProtocolVersion(*blockParams.protocolVersion)
	}

	blockBuilder.Payload(blockParams.payload)

	blockBuilder.SlotCommitment(blockParams.slotCommitment)

	blockBuilder.LatestFinalizedSlot(*blockParams.latestFinalizedSlot)

	blockBuilder.IssuingTime(*blockParams.issuingTime)

	if strongParents, exists := blockParams.references[model.StrongParentType]; exists && len(strongParents) > 0 {
		blockBuilder.StrongParents(strongParents)
	} else {
		return nil, errors.New("cannot create a block without strong parents")
	}

	if weakParents, exists := blockParams.references[model.WeakParentType]; exists {
		blockBuilder.WeakParents(weakParents)
	}

	if shallowLikeParents, exists := blockParams.references[model.ShallowLikeParentType]; exists {
		blockBuilder.ShallowLikeParents(shallowLikeParents)
	}

	blockBuilder.Sign(i.Account.ID(), i.Account.PrivateKey())

	blockBuilder.ProofOfWork(ctx, *blockParams.proofOfWorkDifficulty)

	block, err := blockBuilder.Build()
	if err != nil {
		return nil, errors.Wrap(err, "error building block")
	}

	modelBlock, err := model.BlockFromBlock(block, i.protocol.API())
	if err != nil {
		return nil, errors.Wrap(err, "error serializing block to model block")
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
	triggered := make(chan *blocks.Block, 1)
	exit := make(chan struct{})
	defer close(exit)

	defer evt.Hook(func(eventBlock *blocks.Block) {
		if block.ID() != eventBlock.ID() {
			return
		}
		select {
		case triggered <- eventBlock:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.issueBlock(block); err != nil {
		return errors.Wrapf(err, "failed to issue block %s", block.ID())
	}

	select {
	case <-ctx.Done():
		return errors.Errorf("context canceled whilst waiting for event on block %s", block.ID())
	case <-triggered:
		return nil
	}
}

func (i *BlockIssuer) AttachBlock(ctx context.Context, iotaBlock *iotago.Block) (iotago.BlockID, error) {
	// if anything changes, need to make a new signature
	var resign bool
	protoParams := i.protocol.MainEngineInstance().Storage.Settings().ProtocolParameters()
	targetScore := protoParams.MinPoWScore

	if iotaBlock.ProtocolVersion != protoParams.Version {
		return iotago.EmptyBlockID(), errors.Wrapf(ErrBlockAttacherInvalidBlock, "protocolVersion invalid: %d", iotaBlock.ProtocolVersion)
	}

	if iotaBlock.NetworkID == 0 {
		iotaBlock.NetworkID = protoParams.NetworkID()
		resign = true
	}

	if iotaBlock.IssuingTime.IsZero() {
		iotaBlock.IssuingTime = time.Now()
		resign = true
	}

	if iotaBlock.SlotCommitment == nil {
		iotaBlock.SlotCommitment = i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()
		iotaBlock.LatestFinalizedSlot = i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		resign = true
	}

	switch payload := iotaBlock.Payload.(type) {
	case *iotago.Transaction:
		if payload.Essence.NetworkID != protoParams.NetworkID() {
			return iotago.EmptyBlockID(), errors.WithMessagef(ErrBlockAttacherInvalidBlock, "invalid payload, error: wrong networkID: %d", payload.Essence.NetworkID)
		}
	}

	var references model.ParentReferences
	if len(iotaBlock.StrongParents) == 0 {
		if iotaBlock.Nonce != 0 {
			return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, "no parents were given but nonce was != 0")
		}

		if !i.optsPoWEnabled && targetScore != 0 {
			return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, "no parents given and node PoW is disabled")
		}

		// only allow to update tips during proof of work if no parents were given
		var err error
		references, err = i.getReferences(ctx, iotaBlock.Payload)
		if err != nil {
			return iotago.EmptyBlockID(), errors.WithMessagef(ErrBlockAttacherAttachingNotPossible, "tipselection failed, error: %s", err.Error())
		}

		iotaBlock.StrongParents = references[model.StrongParentType]
		iotaBlock.WeakParents = references[model.WeakParentType]
		iotaBlock.ShallowLikeParents = references[model.ShallowLikeParentType]
		resign = true
	} else {
		references = make(model.ParentReferences)
		references[model.StrongParentType] = iotaBlock.StrongParents
		references[model.WeakParentType] = iotaBlock.WeakParents
		references[model.ShallowLikeParentType] = iotaBlock.ShallowLikeParents
	}

	if err := i.validateReferences(iotaBlock.IssuingTime, iotaBlock.SlotCommitment.Index, references); err != nil {
		return iotago.EmptyBlockID(), errors.WithMessagef(ErrBlockAttacherAttachingNotPossible, "invalid block references, error: %s", err.Error())
	}

	if iotaBlock.IssuerID.Empty() || resign {
		if i.optsIncompleteBlockAccepted {
			iotaBlock.IssuerID = i.Account.ID()

			prvKey := i.Account.PrivateKey()
			signature, err := iotaBlock.Sign(iotago.NewAddressKeysForEd25519Address(iotago.Ed25519AddressFromPubKey(prvKey.Public().(ed25519.PublicKey)), prvKey))
			if err != nil {
				return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, err.Error())
			}

			edSig, isEdSig := signature.(*iotago.Ed25519Signature)
			if !isEdSig {
				return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, "unsupported signature type")
			}

			iotaBlock.Signature = edSig
		} else {
			return iotago.EmptyBlockID(), errors.Wrap(ErrBlockAttacherIncompleteBlockNotAllowed, "signature needed")
		}
	}

	if iotaBlock.Nonce == 0 && targetScore != 0 {
		if i.optsPoWEnabled {
			err := iotaBlock.DoPOW(ctx, float64(targetScore))
			if err != nil {
				return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, err.Error())
			}
		} else {
			return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherPoWNotAvailable, "send a complete block")
		}
	}

	modelBlock, err := model.BlockFromBlock(iotaBlock, i.protocol.API())
	if err != nil {
		return iotago.EmptyBlockID(), errors.Wrap(err, "error serializing block to model block")
	}

	i.events.BlockConstructed.Trigger(modelBlock)

	if err := i.IssueBlockAndAwaitEvent(ctx, modelBlock, i.protocol.Events.Engine.BlockDAG.BlockAttached); err != nil {
		return iotago.EmptyBlockID(), errors.Wrap(err, "error issuing model block")
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
		if b, exists := i.protocol.MainEngineInstance().BlockFromCache(parent); exists {
			if b.IssuingTime().After(issuingTime) {
				return errors.Errorf("cannot issue block if the parents issuingTime is ahead block's issuingTime: %s vs %s", b.IssuingTime(), issuingTime)
			}
			if b.SlotCommitmentID().Index() > slotCommitmentIndex {
				return errors.Errorf("cannot issue block if the commitment is ahead of its parents' commitment: %s vs %s", b.SlotCommitmentID().Index(), slotCommitmentIndex)

			}

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
		references = i.protocol.MainEngineInstance().TipManager.SelectTips(parentsCount)
		if len(references[model.StrongParentType]) > 0 {
			return references, nil
		}

		select {
		case <-interval.C:
			i.events.Error.Trigger(errors.Wrap(err, "could not get references"))
			continue
		case <-timeout.C:
			return nil, errors.Errorf("timeout while trying to select tips and determine references")
		case <-ctx.Done():
			return nil, errors.Errorf("context canceled whilst trying to select tips and determine references: %s", ctx.Err().Error())
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

func WithPoWEnabled(enabled bool) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsPoWEnabled = enabled
	}
}

func WithIncompleteBlockAccepted(accepted bool) options.Option[BlockIssuer] {
	return func(i *BlockIssuer) {
		i.optsIncompleteBlockAccepted = accepted
	}
}
