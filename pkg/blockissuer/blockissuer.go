package blockissuer

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
	ErrBlockAttacherInvalidBlock         = errors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible = errors.New("attaching not possible")
	ErrBlockAttacherPoWNotAvailable      = errors.New("proof of work is not available on this node")
)

// BlockIssuer contains logic to create and issue blocks signed by the given account.
type BlockIssuer struct {
	events *Events

	workerPool *workerpool.WorkerPool

	Account  Account
	protocol *protocol.Protocol

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
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
		return errors.Wrapf(err, "failed to issue block %s", block.ID().String())
	}

	select {
	case <-ctx.Done():
		return errors.Errorf("context canceled whilst waiting for event on block %s", block.ID())
	case <-triggered:
		return nil
	}
}

// CreateBlock creates a new block with the given payload and an optionally defined amount of strong parents.
func (i *BlockIssuer) CreateBlock(ctx context.Context, p iotago.Payload, parentsCount ...int) (*model.Block, error) {
	return i.CreateBlockWithReferences(ctx, p, nil, parentsCount...)
}

// CreateBlockWithReferences creates a new block with the given payload and parent references.
func (i *BlockIssuer) CreateBlockWithReferences(ctx context.Context, p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (*model.Block, error) {
	var err error
	references, err = i.getReferences(ctx, p, references, strongParentsCountOpt...)
	if err != nil {
		return nil, errors.WithMessagef(err, "tipselection failed, error: %s", err.Error())
	}

	slotCommitment := i.protocol.MainEngineInstance().Storage.Settings().LatestCommitment()
	lastFinalizedSlot := i.protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()

	block, err := builder.NewBlockBuilder().
		StrongParents(references[model.StrongParentType]).
		WeakParents(references[model.WeakParentType]).
		ShallowLikeParents(references[model.ShallowLikeParentType]).
		SlotCommitment(slotCommitment.Commitment()).
		LatestFinalizedSlot(lastFinalizedSlot).
		Payload(p).
		Sign(i.Account.ID(), i.Account.PrivateKey()).
		ProofOfWork(ctx, float64(i.protocol.MainEngineInstance().Storage.Settings().ProtocolParameters().MinPoWScore)).
		Build()
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

	if len(iotaBlock.StrongParents) == 0 {
		if iotaBlock.Nonce != 0 {
			return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, "no parents were given but nonce was != 0")
		}

		// only allow to update tips during proof of work if no parents were given
		references, err := i.getReferences(ctx, iotaBlock.Payload, nil)
		if err != nil {
			return iotago.EmptyBlockID(), errors.WithMessagef(ErrBlockAttacherAttachingNotPossible, "tipselection failed, error: %s", err.Error())
		}

		iotaBlock.StrongParents = references[model.StrongParentType]
		iotaBlock.WeakParents = references[model.WeakParentType]
		iotaBlock.ShallowLikeParents = references[model.ShallowLikeParentType]
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

	if iotaBlock.IssuerID.Empty() || resign {
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
	}

	if iotaBlock.Nonce == 0 && targetScore != 0 {
		err := iotaBlock.DoPOW(ctx, float64(targetScore))
		if err != nil {
			return iotago.EmptyBlockID(), errors.WithMessage(ErrBlockAttacherInvalidBlock, err.Error())
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

func (i *BlockIssuer) getReferences(ctx context.Context, p iotago.Payload, references model.ParentReferences, strongParentsCountOpt ...int) (model.ParentReferences, error) {
	strongParentsCount := iotago.BlockMaxParents
	if len(strongParentsCountOpt) > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	var err error
	if references == nil {
		references, err = i.getReferencesWithRetry(ctx, p, strongParentsCount)
		if err != nil {
			return nil, errors.Wrap(err, "error while trying to get references")
		}
	}

	parentsMaxTime := time.Time{}
	parents := lo.Flatten(lo.Map(lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds }))
	for _, parent := range parents {
		if b, exists := i.protocol.MainEngineInstance().BlockFromCache(parent); exists {
			if b.IssuingTime().After(parentsMaxTime) {
				parentsMaxTime = b.IssuingTime()
			}
		}
	}

	if parentsMaxTime.After(time.Now()) {
		return nil, errors.Errorf("cannot issue block if the parents issuingTime is ahead of our local clock: %s vs %s", parentsMaxTime, time.Now())
	}

	return references, nil
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
		references = i.protocol.TipManager.Tips(parentsCount)
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
