package tipselectionv1

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

// PayloadUtils is a utility component that provides helper functions for dealing payloads.
type PayloadUtils struct {
	memPool mempool.MemPool[ledger.BlockVoteRank]
	blocks  *blocks.Blocks
}

// NewPayloadUtils creates a new PayloadUtils instance.
func NewPayloadUtils(memPool mempool.MemPool[ledger.BlockVoteRank], blocks *blocks.Blocks) *PayloadUtils {
	return &PayloadUtils{
		memPool: memPool,
		blocks:  blocks,
	}
}

// UnacceptedInputs returns the unaccepted inputs of a payload.
func (t *PayloadUtils) UnacceptedInputs(payload iotago.Payload) (unacceptedInputs []mempool.StateMetadata, err error) {
	if payload.PayloadType() != iotago.PayloadSignedTransaction {
		return nil, nil
	}

	signedTransaction, isSignedTransaction := payload.(*iotago.SignedTransaction)
	if !isSignedTransaction {
		return nil, ierrors.New("failed to cast payload to signed transaction")
	}

	inputReferences, inputReferencesErr := t.memPool.VM().Inputs(signedTransaction.Transaction)
	if inputReferencesErr != nil {
		return nil, ierrors.Wrap(inputReferencesErr, "failed to retrieve input references")
	}

	for _, inputReference := range inputReferences {
		stateMetadata, stateMetadataErr := t.memPool.StateMetadata(inputReference)
		if stateMetadataErr != nil {
			return nil, ierrors.Wrap(stateMetadataErr, "failed to retrieve state metadata")
		}

		if !stateMetadata.IsAccepted() {
			unacceptedInputs = append(unacceptedInputs, stateMetadata)
		}
	}

	return unacceptedInputs, nil
}

// UnacceptedTransactionDependencies returns the unaccepted transaction dependencies of a payload.
func (t *PayloadUtils) UnacceptedTransactionDependencies(payload iotago.Payload) (dependencies ds.Set[mempool.TransactionMetadata], err error) {
	unacceptedInputs, unacceptedInputsErr := t.UnacceptedInputs(payload)
	if unacceptedInputsErr != nil {
		return nil, ierrors.Wrap(unacceptedInputsErr, "failed to retrieve unaccepted inputs")
	}

	dependencies = ds.NewSet[mempool.TransactionMetadata]()
	for _, unacceptedInput := range unacceptedInputs {
		creatingTransaction := unacceptedInput.CreatingTransaction()
		if creatingTransaction == nil {
			return nil, ierrors.New("unable to reference non-accepted input - creating transaction not found")
		}

		dependencies.Add(creatingTransaction)
	}

	return dependencies, nil
}

// LatestValidAttachment returns the latest valid attachment of a transaction.
func (t *PayloadUtils) LatestValidAttachment(transaction mempool.TransactionMetadata) (latestValidAttachment iotago.BlockID, err error) {
	validAttachments := transaction.ValidAttachments()
	if len(validAttachments) == 0 {
		return iotago.EmptyBlockID, ierrors.Errorf("transaction %s has no valid attachments", transaction.ID())
	}

	var latestValidAttachmentBlock *blocks.Block
	isLaterAttachment := func(block *blocks.Block) bool {
		switch {
		case latestValidAttachmentBlock == nil:
			return true
		case block.ID().Slot() > latestValidAttachmentBlock.ID().Slot():
			return true
		case block.ID().Slot() == latestValidAttachmentBlock.ID().Slot():
			return block.ProtocolBlock().Header.IssuingTime.After(latestValidAttachmentBlock.ProtocolBlock().Header.IssuingTime)
		default:
			return false
		}
	}

	for _, validAttachment := range validAttachments {
		if block, exists := t.blocks.Block(validAttachment); exists && isLaterAttachment(block) {
			latestValidAttachmentBlock = block
		}
	}

	return latestValidAttachmentBlock.ID(), nil
}

// LatestValidAttachments returns the latest valid attachments of a set of transactions.
func (t *PayloadUtils) LatestValidAttachments(transactions ds.Set[mempool.TransactionMetadata]) (latestValidAttachments ds.Set[iotago.BlockID], err error) {
	latestValidAttachments = ds.NewSet[iotago.BlockID]()

	return latestValidAttachments, transactions.ForEach(func(transaction mempool.TransactionMetadata) error {
		latestValidAttachment, latestValidAttachmentErr := t.LatestValidAttachment(transaction)
		if latestValidAttachmentErr == nil {
			latestValidAttachments.Add(latestValidAttachment)
		}

		return latestValidAttachmentErr
	})
}
