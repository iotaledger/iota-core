package ledger

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/booker"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool)
	OnTransactionAttached(callback func(transactionMetadata mempool.TransactionMetadata), opts ...event.Option)
	Output(id iotago.IndexedUTXOReferencer) (*utxoledger.Output, error)
	IsOutputUnspent(outputID iotago.OutputID) (bool, error)
	Spent(outputID iotago.OutputID) (*utxoledger.Spent, error)
	TransactionMetadata(id iotago.TransactionID) (transactionMetadata mempool.TransactionMetadata, exists bool)
	TransactionMetadataByAttachment(blockID iotago.BlockID) (transactionMetadata mempool.TransactionMetadata, exists bool)
	CommitSlot(index iotago.SlotIndex) (stateRoot, mutationRoot, accountRoot iotago.Identifier, err error)
	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, booker.BlockVotePower]
	StateDiffs(index iotago.SlotIndex) (*utxoledger.SlotDiff, error)
	AddUnspentOutput(unspentOutput *utxoledger.Output) error
	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error

	module.Interface
}
