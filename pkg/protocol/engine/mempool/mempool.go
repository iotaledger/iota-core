package mempool

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/spenddag"
	iotago "github.com/iotaledger/iota.go/v4"
)

type MemPool[VoteRank spenddag.VoteRankType[VoteRank]] interface {
	AttachSignedTransaction(signedTransaction SignedTransaction, transaction Transaction, blockID iotago.BlockID) (signedTransactionMetadata SignedTransactionMetadata, err error)

	OnSignedTransactionAttached(callback func(signedTransactionMetadata SignedTransactionMetadata), opts ...event.Option)

	OnTransactionAttached(callback func(metadata TransactionMetadata), opts ...event.Option)

	MarkAttachmentIncluded(blockID iotago.BlockID) bool

	StateMetadata(reference StateReference) (state StateMetadata, err error)

	TransactionMetadata(id iotago.TransactionID) (transaction TransactionMetadata, exists bool)

	VM() VM

	InjectRequestedState(state State)

	TransactionMetadataByAttachment(blockID iotago.BlockID) (transaction TransactionMetadata, exists bool)

	StateDiff(slot iotago.SlotIndex) (StateDiff, error)

	Evict(slot iotago.SlotIndex)

	// Reset resets the component to a clean state as if it was created at the last commitment.
	Reset()
}
