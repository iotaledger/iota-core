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
	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timeutil"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/filter/presolidfilter"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
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

	AccountID  iotago.AccountID
	OutputID   iotago.OutputID
	PublicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey

	events *Events

	workerPool *workerpool.WorkerPool

	optsTipSelectionTimeout       time.Duration
	optsTipSelectionRetryInterval time.Duration
	// optsIncompleteBlockAccepted defines whether the node allows filling in incomplete block and issuing it for user.
	optsIncompleteBlockAccepted bool
	optsRateSetterEnabled       bool
}

func NewBlockIssuer(t *testing.T, name string, keyManager *wallet.KeyManager, accountID iotago.AccountID, validator bool, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	priv, pub := keyManager.KeyPair()

	if accountID == iotago.EmptyAccountID {
		accountID = blake2b.Sum256(pub)
	}
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

func (i *BlockIssuer) BlockIssuerKey() iotago.BlockIssuerKey {
	return iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(i.PublicKey))
}

func (i *BlockIssuer) BlockIssuerKeys() iotago.BlockIssuerKeys {
	return iotago.NewBlockIssuerKeys(i.BlockIssuerKey())
}

// Shutdown shuts down the block issuer.
func (i *BlockIssuer) Shutdown() {
	i.workerPool.Shutdown()
	i.workerPool.ShutdownComplete.Wait()
}

func (i *BlockIssuer) CreateValidationBlock(ctx context.Context, alias string, issuerAccount wallet.Account, node *Node, opts ...options.Option[ValidationBlockParams]) (*blocks.Block, error) {
	blockParams := options.Apply(&ValidationBlockParams{}, opts)

	if blockParams.BlockHeader.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.BlockHeader.IssuingTime = &issuingTime
	}

	apiForBlock, err := i.retrieveAPI(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	if blockParams.BlockHeader.SlotCommitment == nil {
		var err error
		blockParams.BlockHeader.SlotCommitment, err = i.getAddressableCommitment(apiForBlock, *blockParams.BlockHeader.IssuingTime, node)
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
		references, err := i.getReferencesValidationBlock(ctx, node, blockParams.BlockHeader.ParentsCount)
		require.NoError(i.Testing, err)

		blockParams.BlockHeader.References = references
	}

	err = i.setDefaultBlockParams(blockParams.BlockHeader, node)
	require.NoError(i.Testing, err)

	if blockParams.HighestSupportedVersion == nil {
		// We use the latest supported version and not the current one.
		version := node.Protocol.LatestAPI().Version()
		blockParams.HighestSupportedVersion = &version
	}

	if blockParams.ProtocolParametersHash == nil {
		protocolParametersHash, err := apiForBlock.ProtocolParameters().Hash()
		require.NoError(i.Testing, err)

		blockParams.ProtocolParametersHash = &protocolParametersHash
	}

	blockBuilder := builder.NewValidationBlockBuilder(apiForBlock)

	blockBuilder.SlotCommitmentID(blockParams.BlockHeader.SlotCommitment.MustID())
	blockBuilder.LatestFinalizedSlot(*blockParams.BlockHeader.LatestFinalizedSlot)
	blockBuilder.IssuingTime(*blockParams.BlockHeader.IssuingTime)

	strongParents, exists := blockParams.BlockHeader.References[iotago.StrongParentType]
	require.True(i.Testing, exists && len(strongParents) > 0, "block should have strong parents (exists: %t, parents: %s)", exists, strongParents)
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

func (i *BlockIssuer) IssueValidationBlock(ctx context.Context, alias string, node *Node, opts ...options.Option[ValidationBlockParams]) *blocks.Block {
	block, err := i.CreateValidationBlock(ctx, alias, wallet.NewEd25519Account(i.AccountID, i.privateKey), node, opts...)
	require.NoError(i.Testing, err)

	require.NoError(i.Testing, i.IssueBlock(block.ModelBlock(), node))

	validationBlock, _ := block.ValidationBlock()

	node.Protocol.Engines.Main.Get().LogTrace("issued validation block", "blockID", block.ID(), "slot", block.ID().Slot(), "commitment", block.SlotCommitmentID(), "latestFinalizedSlot", block.ProtocolBlock().Header.LatestFinalizedSlot, "version", block.ProtocolBlock().Header.ProtocolVersion, "highestSupportedVersion", validationBlock.HighestSupportedVersion, "hash", validationBlock.ProtocolParametersHash)

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
		references, err := i.getReferencesBasicBlock(ctx, node, blockParams.BlockHeader.ParentsCount)
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
	rmc, err := node.Protocol.Engines.Main.Get().Ledger.RMCManager().RMC(rmcSlot)
	require.NoError(i.Testing, err)

	// only calculate the burned Mana as the last step before signing, so workscore calculation is correct.
	blockBuilder.CalculateAndSetMaxBurnedMana(rmc)

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

	basicBlockBody, is := block.BasicBlock()
	if !is {
		panic("expected basic block")
	}

	node.Protocol.LogTrace("issued block", "blockID", block.ID(), "slot", block.ID().Slot(), "MaxBurnedMana", basicBlockBody.MaxBurnedMana, "commitment", block.SlotCommitmentID(), "latestFinalizedSlot", block.ProtocolBlock().Header.LatestFinalizedSlot, "version", block.ProtocolBlock().Header.ProtocolVersion)

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

	defer node.Protocol.Events.Engine.PreSolidFilter.BlockPreFiltered.Hook(func(event *presolidfilter.BlockPreFilteredEvent) {
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
		latestFinalizedSlot := node.Protocol.Engines.Main.Get().SyncManager.LatestFinalizedSlot()
		blockParams.LatestFinalizedSlot = &latestFinalizedSlot
	}

	if blockParams.Issuer == nil {
		blockParams.Issuer = wallet.NewEd25519Account(i.AccountID, i.privateKey)
	} else if blockParams.Issuer.ID() != i.AccountID {
		return ierrors.Errorf("provided issuer account %s, but issuer provided in the block params is different %s", i.AccountID, blockParams.Issuer.ID())
	}

	if !blockParams.SkipReferenceValidation {
		if err := i.validateReferences(*blockParams.IssuingTime, blockParams.SlotCommitment.Slot, blockParams.References, node); err != nil {
			return ierrors.Wrap(err, "block references invalid")
		}
	}

	return nil
}

func (i *BlockIssuer) getAddressableCommitment(currentAPI iotago.API, blockIssuingTime time.Time, node *Node) (*iotago.Commitment, error) {
	protoParams := currentAPI.ProtocolParameters()
	blockSlot := currentAPI.TimeProvider().SlotFromTime(blockIssuingTime)

	commitment := node.Protocol.Engines.Main.Get().SyncManager.LatestCommitment().Commitment()

	if blockSlot > commitment.Slot+protoParams.MaxCommittableAge() {
		return nil, ierrors.Wrapf(ErrBlockTooRecent, "can't issue block: block slot %d is too far in the future, latest commitment is %d", blockSlot, commitment.Slot)
	}

	if blockSlot < commitment.Slot+protoParams.MinCommittableAge() {
		if blockSlot < protoParams.MinCommittableAge() || commitment.Slot < protoParams.MinCommittableAge() {
			return commitment, nil
		}

		commitmentSlot := commitment.Slot - protoParams.MinCommittableAge()
		loadedCommitment, err := node.Protocol.Engines.Main.Get().Storage.Commitments().Load(commitmentSlot)
		if err != nil {
			return nil, ierrors.Wrapf(err, "error loading valid commitment of slot %d according to minCommittableAge from storage", commitmentSlot)
		}

		return loadedCommitment.Commitment(), nil
	}

	return commitment, nil
}

func (i *BlockIssuer) getReferencesBasicBlock(ctx context.Context, node *Node, strongParentsCountOpt ...int) (model.ParentReferences, error) {
	strongParentsCount := iotago.BasicBlockMaxParents
	if len(strongParentsCountOpt) > 0 && strongParentsCountOpt[0] > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	return i.getReferencesWithRetry(ctx, strongParentsCount, node)
}

func (i *BlockIssuer) getReferencesValidationBlock(ctx context.Context, node *Node, strongParentsCountOpt ...int) (model.ParentReferences, error) {
	strongParentsCount := iotago.ValidationBlockMaxParents
	if len(strongParentsCountOpt) > 0 && strongParentsCountOpt[0] > 0 {
		strongParentsCount = strongParentsCountOpt[0]
	}

	return i.getReferencesWithRetry(ctx, strongParentsCount, node)
}

func (i *BlockIssuer) validateReferences(issuingTime time.Time, slotCommitmentIndex iotago.SlotIndex, references model.ParentReferences, node *Node) error {
	for _, parent := range lo.Flatten(lo.Map(lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds })) {
		b, exists := node.Protocol.Engines.Main.Get().BlockFromCache(parent)
		if !exists {
			return ierrors.Errorf("cannot issue block if the parents are not known: %s", parent)
		}

		if b.IssuingTime().After(issuingTime) {
			return ierrors.Errorf("cannot issue block if the parents issuingTime is ahead block's issuingTime: %s vs %s", b.IssuingTime(), issuingTime.UTC())
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
		_ = node.Protocol.Engines.Main.Get().Storage.Settings().SetLatestIssuedValidationBlock(block)
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
func (i *BlockIssuer) getReferencesWithRetry(ctx context.Context, parentsCount int, node *Node) (references model.ParentReferences, err error) {
	timeout := time.NewTimer(i.optsTipSelectionTimeout)
	interval := time.NewTicker(i.optsTipSelectionRetryInterval)
	defer timeutil.CleanupTimer(timeout)
	defer timeutil.CleanupTicker(interval)

	for {
		references = node.Protocol.Engines.Main.Get().TipSelection.SelectTips(parentsCount)
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
