package mock

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"

	hiveEd25519 "github.com/iotaledger/hive.go/crypto/ed25519"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/serializer/v2/serix"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
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

	keyManager *wallet.KeyManager
	client     Client

	AccountData AccountData
}

func NewBlockIssuer(t *testing.T, name string, keyManager *wallet.KeyManager, client Client, addressIndex uint32, accountID iotago.AccountID, validator bool, opts ...options.Option[BlockIssuer]) *BlockIssuer {
	_, pub := keyManager.KeyPair(addressIndex)

	if accountID == iotago.EmptyAccountID {
		accountID = blake2b.Sum256(pub)
	}
	accountID.RegisterAlias(name)

	return options.Apply(&BlockIssuer{
		Testing:    t,
		Name:       name,
		Validator:  validator,
		keyManager: keyManager,
		client:     client,
		AccountData: AccountData{
			ID:           accountID,
			AddressIndex: addressIndex,
		},
	}, opts)
}

func (i *BlockIssuer) BlockIssuerKey() iotago.BlockIssuerKey {
	_, pub := i.keyManager.KeyPair()
	return iotago.Ed25519PublicKeyHashBlockIssuerKeyFromPublicKey(hiveEd25519.PublicKey(pub))
}

func (i *BlockIssuer) BlockIssuerKeys() iotago.BlockIssuerKeys {
	return iotago.NewBlockIssuerKeys(i.BlockIssuerKey())
}

func (i *BlockIssuer) Address() iotago.Address {
	_, pub := i.keyManager.KeyPair()
	return iotago.Ed25519AddressFromPubKey(pub)
}

func (i *BlockIssuer) CreateValidationBlock(ctx context.Context, alias string, opts ...options.Option[ValidationBlockParams]) (*blocks.Block, error) {
	blockParams := options.Apply(&ValidationBlockParams{}, opts)

	if blockParams.BlockHeader.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.BlockHeader.IssuingTime = &issuingTime
	}

	apiForBlock := i.retrieveAPI(blockParams.BlockHeader)
	protoParams := apiForBlock.ProtocolParameters()
	blockIssuanceInfo := i.client.BlockIssuance(ctx)
	if blockParams.BlockHeader.SlotCommitment == nil {
		commitment := blockIssuanceInfo.LatestCommitment
		blockSlot := apiForBlock.TimeProvider().SlotFromTime(*blockParams.BlockHeader.IssuingTime)
		if blockSlot > commitment.Slot+protoParams.MaxCommittableAge() {
			return nil, ierrors.Wrapf(ErrBlockTooRecent, "can't issue block: block slot %d is too far in the future, latest commitment is %d", blockSlot, commitment.Slot)
		}

		if blockSlot < commitment.Slot+protoParams.MinCommittableAge() &&
			blockSlot > protoParams.MinCommittableAge() &&
			commitment.Slot > protoParams.MinCommittableAge() {

			commitmentSlot := commitment.Slot - protoParams.MinCommittableAge()
			commitment = i.client.CommitmentByIndex(ctx, commitmentSlot)
		}

		blockParams.BlockHeader.SlotCommitment = commitment
		// TODO: check if this is still necessary and, if so, if this is implemented in inx-validator
		// if err != nil && ierrors.Is(err, ErrBlockTooRecent) {
		// 	commitment, parentID, err := i.reviveChain(*blockParams.BlockHeader.IssuingTime, node)
		// 	if err != nil {
		// 		return nil, ierrors.Wrap(err, "failed to revive chain")
		// 	}
		// 	blockParams.BlockHeader.SlotCommitment = commitment
		// 	blockParams.BlockHeader.References = make(model.ParentReferences)
		// 	blockParams.BlockHeader.References[iotago.StrongParentType] = []iotago.BlockID{parentID}

		// } else if err != nil {
		// 	return nil, ierrors.Wrap(err, "error getting commitment")
		// }
	}

	if blockParams.BlockHeader.References == nil {
		blockParams.BlockHeader.References = referencesFromBlockIssuanceResponse(blockIssuanceInfo)
	}

	err := i.setDefaultBlockParams(ctx, blockParams.BlockHeader)
	require.NoError(i.Testing, err)

	if blockParams.HighestSupportedVersion == nil {
		// We use the latest supported version and not the current one.
		version := i.client.LatestAPI().Version()
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

	priv, _ := i.keyManager.KeyPair(i.AccountData.AddressIndex)
	blockBuilder.Sign(i.AccountData.ID, priv)

	block, err := blockBuilder.Build()
	require.NoError(i.Testing, err)

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	require.NoError(i.Testing, err)

	modelBlock.ID().RegisterAlias(alias)

	return blocks.NewBlock(modelBlock), nil
}

func referencesFromBlockIssuanceResponse(response *api.IssuanceBlockHeaderResponse) model.ParentReferences {
	references := make(model.ParentReferences)
	references[iotago.StrongParentType] = response.StrongParents
	references[iotago.WeakParentType] = response.WeakParents
	references[iotago.ShallowLikeParentType] = response.ShallowLikeParents
	return references
}

func (i *BlockIssuer) IssueValidationBlock(ctx context.Context, alias string, opts ...options.Option[ValidationBlockParams]) (*blocks.Block, error) {
	block, err := i.CreateValidationBlock(ctx, alias, opts...)
	require.NoError(i.Testing, err)

	err = i.SubmitBlock(ctx, block.ModelBlock())

	return block, err
}

// CreateBlock creates a new block with the options.
func (i *BlockIssuer) CreateBasicBlock(ctx context.Context, alias string, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	blockParams := options.Apply(&BasicBlockParams{}, opts)

	if blockParams.BlockHeader.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.BlockHeader.IssuingTime = &issuingTime
	}
	blockIssuanceInfo := i.client.BlockIssuance(ctx)

	if blockParams.BlockHeader.References == nil {
		blockParams.BlockHeader.References = referencesFromBlockIssuanceResponse(blockIssuanceInfo)
	}

	err := i.setDefaultBlockParams(ctx, blockParams.BlockHeader)
	require.NoError(i.Testing, err)

	api := i.client.APIForTime(*blockParams.BlockHeader.IssuingTime)
	blockBuilder := builder.NewBasicBlockBuilder(api)

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

	blockBuilder.Payload(blockParams.Payload)

	// use the rmc corresponding to the commitment used in the block
	blockBuilder.CalculateAndSetMaxBurnedMana(blockIssuanceInfo.LatestCommitment.ReferenceManaCost)

	priv, _ := i.keyManager.KeyPair(i.AccountData.AddressIndex)
	blockBuilder.Sign(i.AccountData.ID, priv)

	block, err := blockBuilder.Build()
	require.NoError(i.Testing, err)

	// Make sure we only create syntactically valid blocks.
	modelBlock, err := model.BlockFromBlock(block, serix.WithValidation())
	require.NoError(i.Testing, err)

	modelBlock.ID().RegisterAlias(alias)

	return blocks.NewBlock(modelBlock), err
}

func (i *BlockIssuer) IssueBasicBlock(ctx context.Context, alias string, opts ...options.Option[BasicBlockParams]) (*blocks.Block, error) {
	block, err := i.CreateBasicBlock(ctx, alias, opts...)
	if err != nil {
		return nil, err
	}

	err = i.SubmitBlock(ctx, block.ModelBlock())

	return block, err
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
			lo.PanicOnErr(i.IssueValidationBlock(ctx, blockAlias,
				WithValidationBlockHeaderOptions(
					WithIssuingTime(issuingTime.Add(timeOffset)),
				),
			))

			counter++
			time.Sleep(1 * time.Second)
		}
	}()
}

func (i *BlockIssuer) setDefaultBlockParams(ctx context.Context, blockParams *BlockHeaderParams) error {
	if blockParams.IssuingTime == nil {
		issuingTime := time.Now().UTC()
		blockParams.IssuingTime = &issuingTime
	}

	issuanceInfo := i.client.BlockIssuance(ctx)

	if blockParams.SlotCommitment == nil {
		blockParams.SlotCommitment = issuanceInfo.LatestCommitment
	}

	if blockParams.LatestFinalizedSlot == nil {
		blockParams.LatestFinalizedSlot = &issuanceInfo.LatestFinalizedSlot
	}

	if blockParams.Issuer == nil {
		priv, _ := i.keyManager.KeyPair(i.AccountData.AddressIndex)
		blockParams.Issuer = wallet.NewEd25519Account(i.AccountData.ID, priv)
	} else if blockParams.Issuer.ID() != i.AccountData.ID {
		return ierrors.Errorf("provided issuer account %s, but issuer provided in the block params is different %s", i.AccountData.ID, blockParams.Issuer.ID())
	}

	if blockParams.ReferenceValidation {
		if err := i.validateReferences(ctx, *blockParams.IssuingTime, blockParams.SlotCommitment.Slot, blockParams.References); err != nil {
			return ierrors.Wrap(err, "block references invalid")
		}
	}

	return nil
}

func (i *BlockIssuer) validateReferences(ctx context.Context, issuingTime time.Time, slotCommitmentIndex iotago.SlotIndex, references model.ParentReferences) error {
	for _, parent := range lo.Flatten(lo.Map(lo.Values(references), func(ds iotago.BlockIDs) []iotago.BlockID { return ds })) {
		b, err := i.client.BlockByBlockID(ctx, parent)
		if err != nil {
			return ierrors.Wrapf(err, "cannot issue block if the parents could not be retrieved: %s", parent)
		}

		if b.Header.IssuingTime.After(issuingTime) {
			return ierrors.Errorf("cannot issue block if the parents issuingTime is ahead block's issuingTime: %s vs %s", b.Header.IssuingTime, issuingTime.UTC())
		}
		if b.Header.SlotCommitmentID.Slot() > slotCommitmentIndex {
			return ierrors.Errorf("cannot issue block if the commitment is ahead of its parents' commitment: %s vs %s", b.Header.SlotCommitmentID.Slot(), slotCommitmentIndex)
		}
	}

	return nil
}

func (i *BlockIssuer) SubmitBlock(ctx context.Context, block *model.Block) error {
	return lo.Return2(i.client.SubmitBlock(ctx, block.ProtocolBlock()))
}

func (i *BlockIssuer) SubmitBlockWithoutAwaitingBooking(block *model.Block, node *Node) error {
	if err := node.RequestHandler.SubmitBlockWithoutAwaitingBooking(block); err != nil {
		return err
	}

	if _, isValidationBlock := block.ValidationBlock(); isValidationBlock {
		_ = node.Protocol.Engines.Main.Get().Storage.Settings().SetLatestIssuedValidationBlock(block)
	}

	return nil
}

func (i *BlockIssuer) CopyIdentityFromBlockIssuer(otherBlockIssuer *BlockIssuer) {
	i.keyManager = otherBlockIssuer.keyManager
	i.AccountData = otherBlockIssuer.AccountData
	i.Validator = otherBlockIssuer.Validator
}

func (i *BlockIssuer) retrieveAPI(blockParams *BlockHeaderParams) iotago.API {
	if blockParams.ProtocolVersion != nil {
		return i.client.APIForVersion(*blockParams.ProtocolVersion)
	}

	// It is crucial to get the API from the issuing time/slot as that defines the version with which the block should be issued.
	return i.client.APIForTime(*blockParams.IssuingTime)
}
