package mock

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	"github.com/iotaledger/hive.go/core/safemath"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
)

var (
	ErrBlockAttacherInvalidBlock              = ierrors.New("invalid block")
	ErrBlockAttacherAttachingNotPossible      = ierrors.New("attaching not possible")
	ErrBlockAttacherIncompleteBlockNotAllowed = ierrors.New("incomplete block is not allowed on this node")
	ErrBlockTooRecent                         = ierrors.New("block is too recent compared to latest commitment")
)

// TODO: make sure an honest validator does not issue blocks within the same slot ratification period in two conflicting chains.
//  - this can be achieved by remembering the last issued block together with the engine name/chain.
//  - if the engine name/chain is the same we can always issue a block.
//  - if the engine name/chain is different we need to make sure to wait "slot ratification" slots.

// BlockIssuer contains logic to create and issue blocks signed by the given account.
type BlockIssuer struct {
	Testing *testing.T

	Name      string
	Validator bool

	events *Events

	workerPool *workerpool.WorkerPool

	privateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	AccountID  iotago.AccountID

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
	// optsIncompleteBlockAccepted defines whether the node allows filling in incomplete block and issuing it for user.
	optsIncompleteBlockAccepted bool
	optsRateSetterEnabled       bool
}

func NewBlockIssuer(t *testing.T, name string, validator bool, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	accountID := iotago.AccountID(blake2b.Sum256(pub))
	accountID.RegisterAlias(name)

	return options.Apply(&BlockIssuer{
		Testing:                       t,
		Name:                          name,
		Validator:                     validator,
		events:                        NewEvents(),
		workerPool:                    workerpool.New("BlockIssuer"),
		privateKey:                    priv,
		PublicKey:                     pub,
		AccountID:                     accountID,
		optsIncompleteBlockAccepted:   false,
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

func (i *BlockIssuer) CreateValidationBlock(ctx context.Context, alias string, issuerAccount Account, node *Node, opts ...options.Option[ValidatorBlockParams]) (*blocks.Block, error) {
	blockParams := options.Apply(&ValidatorBlockParams{}, opts)

	if blockParams.BlockHeader.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.BlockHeader.IssuingTime = &issuingTime
	}

	currentAPI := node.Protocol.APIForTime(*blockParams.BlockHeader.IssuingTime)

	if blockParams.BlockHeader.SlotCommitment == nil {
		var err error
		blockParams.BlockHeader.SlotCommitment, err = i.getAddressableCommitment(currentAPI, *blockParams.BlockHeader.IssuingTime, node)
		if err != nil && ierrors.Is(err, ErrBlockTooRecent) {
			commitment, parentID, err := i.reviveChain(*blockParams.BlockHeader.IssuingTime, node)
			if err != nil {
				return nil, ierrors.Wrap(err, "failed to revive chain")
			}
			blockParams.BlockHeader.SlotCommitment = commitment
			blockParams.BlockHeader.References = make(model.ParentReferences)
			blockParams.BlockHeader.References[iotago.StrongParentType] = []iotago.BlockID{parentID}

		} else if err != nil {
			return nil, ierrors.Wrap(err, "error getting commitment")
		}
	}

	if blockParams.BlockHeader.References == nil {
		// TODO: change this to get references for validator block
		references, err := i.getReferences(ctx, nil, node, blockParams.BlockHeader.ParentsCount)
		require.NoError(i.Testing, err)

		blockParams.BlockHeader.References = references
	}

	err := i.setDefaultBlockParams(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	if blockParams.HighestSupportedVersion == nil {
		// We use the latest supported version and not the current one.
		version := node.Protocol.LatestAPI().Version()
		blockParams.HighestSupportedVersion = &version
	}

	if blockParams.ProtocolParametersHash == nil {
		protocolParametersHash, err := currentAPI.ProtocolParameters().Hash()
		require.NoError(i.Testing, err)

		blockParams.ProtocolParametersHash = &protocolParametersHash
	}

	api, err := i.retrieveAPI(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	blockBuilder := builder.NewValidationBlockBuilder(api)

	blockBuilder.SlotCommitmentID(blockParams.BlockHeader.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.BlockHeader.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.BlockHeader.IssuingTime)

	strongParents, exists := blockParams.BlockHeader.References[iotago.StrongParentType]
	require.True(i.Testing, exists && len(strongParents) > 0)
	blockBuilder.StrongParents(strongParents)

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
	require.NoError(i.Testing, err)

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	require.NoError(i.Testing, err)

	i.events.BlockConstructed.Trigger(modelBlock)

	modelBlock.ID().RegisterAlias(alias)

	return blocks.NewBlock(modelBlock), nil
}

func (i *BlockIssuer) IssueValidationBlock(ctx context.Context, alias string, node *Node, opts ...options.Option[ValidatorBlockParams]) *blocks.Block {
	block, err := i.CreateValidationBlock(ctx, alias, NewEd25519Account(i.AccountID, i.privateKey), node, opts...)
	require.NoError(i.Testing, err)

	require.NoError(i.Testing, i.IssueBlock(block.ModelBlock(), node))

	validationBlock, _ := block.ValidationBlock()

	fmt.Printf("Issued ValidationBlock: %s - slot %d - commitment %s %d - latest finalized slot %d - version: %d - highestSupportedVersion: %d, hash: %s\n", block.ID(), block.ID().Slot(), block.SlotCommitmentID(), block.SlotCommitmentID().Slot(), block.ProtocolBlock().LatestFinalizedSlot, block.ProtocolBlock().ProtocolVersion, validationBlock.HighestSupportedVersion, validationBlock.ProtocolParametersHash)

	return block
}

func (i *BlockIssuer) retrieveAPI(blockParams *BlockHeaderParams, node *Node) (iotago.API, error) {
	if blockParams.ProtocolVersion != nil {
		return node.Protocol.APIForVersion(*blockParams.ProtocolVersion)
	}

	// It is crucial to get the API from the issuing time/slot as that defines the version with which the block should be issued.
	return node.Protocol.APIForTime(*blockParams.IssuingTime), nil
}

// CreateBlock creates a new block with the options.
func (i *BlockIssuer) CreateBasicBlock(ctx context.Context, alias string, node *Node, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	blockParams := options.Apply(&BasicBlockParams{}, opts)

	if blockParams.BlockHeader.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.BlockHeader.IssuingTime = &issuingTime
	}
	currentAPI := node.Protocol.APIForTime(*blockParams.BlockHeader.IssuingTime)

	if blockParams.BlockHeader.SlotCommitment == nil {
		var err error
		blockParams.BlockHeader.SlotCommitment, err = i.getAddressableCommitment(currentAPI, *blockParams.BlockHeader.IssuingTime, node)
		if err != nil {
			return nil, ierrors.Wrap(err, "error getting commitment")
		}
	}

	if blockParams.BlockHeader.References == nil {
		references, err := i.getReferences(ctx, blockParams.Payload, node, blockParams.BlockHeader.ParentsCount)
		require.NoError(i.Testing, err)
		blockParams.BlockHeader.References = references
	}

	err := i.setDefaultBlockParams(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	api, err := i.retrieveAPI(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	blockBuilder := builder.NewBasicBlockBuilder(api)

	blockBuilder.SlotCommitmentID(blockParams.BlockHeader.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.BlockHeader.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.BlockHeader.IssuingTime)
	strongParents, exists := blockParams.BlockHeader.References[iotago.StrongParentType]
	require.True(i.Testing, exists && len(strongParents) > 0)
	blockBuilder.StrongParents(strongParents)

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
	rmc, err := node.Protocol.MainEngineInstance().Ledger.RMCManager().RMC(rmcSlot)
	require.NoError(i.Testing, err)

	// only set the burned Mana as the last step before signing, so workscore calculation is correct.
	blockBuilder.MaxBurnedMana(rmc)

	blockBuilder.Sign(i.AccountID, i.privateKey)

	block, err := blockBuilder.Build()
	require.NoError(i.Testing, err)

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	require.NoError(i.Testing, err)

	i.events.BlockConstructed.Trigger(modelBlock)

	modelBlock.ID().RegisterAlias(alias)

	return blocks.NewBlock(modelBlock), err
}

func (i *BlockIssuer) IssueBasicBlock(ctx context.Context, alias string, node *Node, opts ...options.Option[BasicBlockParams]) *blocks.Block {
	block, err := i.CreateBasicBlock(ctx, alias, node, opts...)
	require.NoError(i.Testing, err)

	require.NoErrorf(i.Testing, i.IssueBlock(block.ModelBlock(), node), "%s > failed to issue block with alias %s", i.Name, alias)

	fmt.Printf("%s > Issued block: %s - slot %d - commitment %s %d - latest finalized slot %d\n", i.Name, block.ID(), block.ID().Slot(), block.SlotCommitmentID(), block.SlotCommitmentID().Slot(), block.ProtocolBlock().LatestFinalizedSlot)

	return block
}

func (i *BlockIssuer) IssueActivity(ctx context.Context, wg *sync.WaitGroup, startSlot iotago.SlotIndex, node *Node) {
	issuingTime := node.Protocol.APIForSlot(startSlot).TimeProvider().SlotStartTime(startSlot)
	start := time.Now()

	wg.Add(1)
	go func() {
		defer wg.Done()

		fmt.Println(i.Name, "> Starting activity")
		var counter int
		for {
			if ctx.Err() != nil {
				fmt.Println(i.Name, "> Stopped activity due to canceled context:", ctx.Err())
				return
			}

			blockAlias := fmt.Sprintf("%s-activity.%d", i.Name, counter)
			timeOffset := time.Since(start)
			i.IssueValidationBlock(ctx, blockAlias,
				node,
				WithValidationBlockHeaderOptions(
					WithIssuingTime(issuingTime.Add(timeOffset)),
				),
			)

			counter++
			time.Sleep(1 * time.Second)
		}
	}()
}

// IssueBlockAndAwaitEvent submits a block to be processed and waits for the event to be triggered.
func (i *BlockIssuer) IssueBlockAndAwaitEvent(ctx context.Context, block *model.Block, node *Node, evt *event.Event1[*blocks.Block]) error {
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

	defer node.Protocol.Events.Engine.Filter.BlockPreFiltered.Hook(func(event *filter.BlockPreFilteredEvent) {
		if block.ID() != event.Block.ID() {
			return
		}
		select {
		case triggered <- event.Reason:
		case <-exit:
		}
	}, event.WithWorkerPool(i.workerPool)).Unhook()

	if err := i.IssueBlock(block, node); err != nil {
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

func (i *BlockIssuer) AttachBlock(ctx context.Context, iotaBlock *iotago.ProtocolBlock, node *Node, optIssuerAccount ...Account) (iotago.BlockID, error) {
	// if anything changes, need to make a new signature
	var resign bool

	apiForVersion, err := node.Protocol.APIForVersion(iotaBlock.ProtocolVersion)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "protocolVersion invalid: %d", iotaBlock.ProtocolVersion)
	}

	protoParams := apiForVersion.ProtocolParameters()

	if iotaBlock.NetworkID == 0 {
		iotaBlock.NetworkID = protoParams.NetworkID()
		resign = true
	}

	if iotaBlock.SlotCommitmentID == iotago.EmptyCommitmentID {
		iotaBlock.SlotCommitmentID = node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment().MustID()
		iotaBlock.LatestFinalizedSlot = node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		resign = true
	}

	switch innerBlock := iotaBlock.Block.(type) {
	case *iotago.BasicBlock:
		switch payload := innerBlock.Payload.(type) {
		case *iotago.SignedTransaction:
			if payload.Transaction.NetworkID != protoParams.NetworkID() {
				return iotago.EmptyBlockID, ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "invalid payload, error: wrong networkID: %d", payload.Transaction.NetworkID)
			}
		}

		if len(iotaBlock.Parents()) == 0 {
			references, referencesErr := i.getReferences(ctx, innerBlock.Payload, node)
			if referencesErr != nil {
				return iotago.EmptyBlockID, ierrors.Wrapf(ErrBlockAttacherAttachingNotPossible, "tipselection failed, error: %w", referencesErr)
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

	if err = i.validateReferences(iotaBlock.IssuingTime, iotaBlock.SlotCommitmentID.Slot(), references, node); err != nil {
		return iotago.EmptyBlockID, ierrors.Wrapf(ErrBlockAttacherAttachingNotPossible, "invalid block references, error: %w", err)
	}

	if basicBlock, isBasicBlock := iotaBlock.Block.(*iotago.BasicBlock); isBasicBlock && basicBlock.MaxBurnedMana == 0 {
		rmcSlot, err := safemath.SafeSub(apiForVersion.TimeProvider().SlotFromTime(iotaBlock.IssuingTime), apiForVersion.ProtocolParameters().MaxCommittableAge())
		if err != nil {
			rmcSlot = 0
		}
		rmc, err := node.Protocol.MainEngineInstance().Ledger.RMCManager().RMC(rmcSlot)
		if err != nil {
			return iotago.EmptyBlockID, ierrors.Wrapf(err, "error loading commitment of slot %d from storage to get RMC", rmcSlot)
		}

		// only set the burned Mana as the last step before signing, so workscore calculation is correct.
		basicBlock.MaxBurnedMana, err = basicBlock.ManaCost(rmc, apiForVersion.ProtocolParameters().WorkScoreStructure())
		if err != nil {
			return iotago.EmptyBlockID, ierrors.Wrapf(err, "could not calculate Mana cost for block")
		}
		resign = true
	}

	if iotaBlock.IssuerID.Empty() || resign {
		if i.optsIncompleteBlockAccepted && len(optIssuerAccount) > 0 {
			issuerAccount := optIssuerAccount[0]
			iotaBlock.IssuerID = issuerAccount.ID()

			signature, signatureErr := iotaBlock.Sign(iotago.NewAddressKeysForEd25519Address(issuerAccount.Address().(*iotago.Ed25519Address), issuerAccount.PrivateKey()))
			if signatureErr != nil {
				return iotago.EmptyBlockID, ierrors.Wrapf(ErrBlockAttacherInvalidBlock, "%w", signatureErr)
			}

			edSig, isEdSig := signature.(*iotago.Ed25519Signature)
			if !isEdSig {
				return iotago.EmptyBlockID, ierrors.Wrap(ErrBlockAttacherInvalidBlock, "unsupported signature type")
			}

			iotaBlock.Signature = edSig
		} else {
			return iotago.EmptyBlockID, ierrors.Wrap(ErrBlockAttacherIncompleteBlockNotAllowed, "signature needed")
		}
	}

	modelBlock, err := model.BlockFromBlock(iotaBlock)
	if err != nil {
		return iotago.EmptyBlockID, ierrors.Wrap(err, "error serializing block to model block")
	}

	if !i.optsRateSetterEnabled || node.Protocol.MainEngineInstance().Scheduler.IsBlockIssuerReady(modelBlock.ProtocolBlock().IssuerID) {
		i.events.BlockConstructed.Trigger(modelBlock)

		if err = i.IssueBlockAndAwaitEvent(ctx, modelBlock, node, node.Protocol.Events.Engine.BlockDAG.BlockAttached); err != nil {
			return iotago.EmptyBlockID, ierrors.Wrap(err, "error issuing model block")
		}
	}

	return modelBlock.ID(), nil
}

func (i *BlockIssuer) setDefaultBlockParams(blockParams *BlockHeaderParams, node *Node) error {
	if blockParams.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.IssuingTime = &issuingTime
	}

	if blockParams.SlotCommitment == nil {
		var err error
		currentAPI := node.Protocol.APIForTime(*blockParams.IssuingTime)
		blockParams.SlotCommitment, err = i.getAddressableCommitment(currentAPI, *blockParams.IssuingTime, node)
		if err != nil {
			return ierrors.Wrap(err, "error getting commitment")
		}
	}

	if blockParams.LatestFinalizedSlot == nil {
		latestFinalizedSlot := node.Protocol.MainEngineInstance().Storage.Settings().LatestFinalizedSlot()
		blockParams.LatestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.Issuer == nil {
		blockParams.Issuer = NewEd25519Account(i.AccountID, i.privateKey)
	} else if blockParams.Issuer.ID() != i.AccountID {
		return ierrors.Errorf("provided issuer account %s, but issuer provided in the block params is different %s", i.AccountID, blockParams.Issuer.ID())
	}

	if err := i.validateReferences(*blockParams.IssuingTime, blockParams.SlotCommitment.Slot, blockParams.References, node); err != nil {
		return ierrors.Wrap(err, "block references invalid")
	}

	return nil
}

func (i *BlockIssuer) getAddressableCommitment(currentAPI iotago.API, blockIssuingTime time.Time, node *Node) (*iotago.Commitment, error) {
	protoParams := currentAPI.ProtocolParameters()
	blockSlot := currentAPI.TimeProvider().SlotFromTime(blockIssuingTime)

	commitment := node.Protocol.MainEngineInstance().Storage.Settings().LatestCommitment().Commitment()

	if blockSlot > commitment.Slot+protoParams.MaxCommittableAge() {
		return nil, ierrors.Wrapf(ErrBlockTooRecent, "can't issue block: block slot %d is too far in the future, latest commitment is %d", blockSlot, commitment.Slot)
	}

	if blockSlot < commitment.Slot+protoParams.MinCommittableAge() {
		if blockSlot < protoParams.MinCommittableAge() || commitment.Slot < protoParams.MinCommittableAge() {
			return commitment, nil
		}

		commitmentSlot := commitment.Slot - protoParams.MinCommittableAge()
		loadedCommitment, err := node.Protocol.MainEngineInstance().Storage.Commitments().Load(commitmentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error loading valid commitment of slot %d according to minCommittableAge from storage", commitmentSlot)
		}

		return loadedCommitment.Commitment(), nil
	}

	return commitment, nil
}

func (i *BlockIssuer) getReferences(ctx context.Context, p iotago.Payload, node *Node, strongParentsCountOpt ...int) (model.ParentReferences, error) {
	strongParentsCount := iotago.BlockMaxParents
	if len(strongParentsCountOpt) > 0 && strongParentsCountOpt[0] > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	return i.getReferencesWithRetry(ctx, p, strongParentsCount, node)
}

func (i *BlockIssuer) validateReferences(issuingTime time.Time, slotCommitmentIndex iotago.SlotIndex, references model.ParentReferences, node *Node) error {
	for _, parent := range lo.Flatten(lo.Map(lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds })) {
		b, exists := node.Protocol.MainEngineInstance().BlockFromCache(parent)
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

func (i *BlockIssuer) IssueBlock(block *model.Block, node *Node) error {
	if err := node.Protocol.IssueBlock(block); err != nil {
		return err
	}

	if _, isValidationBlock := block.ValidationBlock(); isValidationBlock {
		_ = node.Protocol.MainEngineInstance().Storage.Settings().SetLatestIssuedValidationBlock(block)
	}

	i.events.BlockIssued.Trigger(block)

	return nil
}

func (i *BlockIssuer) CopyIdentityFromBlockIssuer(otherBlockIssuer *BlockIssuer) {
	i.privateKey = otherBlockIssuer.privateKey
	i.PublicKey = otherBlockIssuer.PublicKey
	i.AccountID = otherBlockIssuer.AccountID
	i.Validator = otherBlockIssuer.Validator
}

// getReferencesWithRetry tries to get references for the given payload. If it fails, it will retry at regular intervals until
// the timeout is reached.
func (i *BlockIssuer) getReferencesWithRetry(ctx context.Context, _ iotago.Payload, parentsCount int, node *Node) (references model.ParentReferences, err error) {
	timeout := time.NewTimer(i.optsTipSelectionTimeout)
	interval := time.NewTicker(i.optsTipSelectionRetryInterval)
	defer timeutil.CleanupTimer(timeout)
	defer timeutil.CleanupTicker(interval)

	for {
		references = node.Protocol.MainEngineInstance().TipSelection.SelectTips(parentsCount)
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
