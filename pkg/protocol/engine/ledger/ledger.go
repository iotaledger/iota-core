package ledger

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool)
	OnTransactionAttached(callback func(transactionMetadata mempool.TransactionMetadata), opts ...event.Option)
	TransactionMetadata(id iotago.TransactionID) (transactionMetadata mempool.TransactionMetadata, exists bool)
	TransactionMetadataByAttachment(blockID iotago.BlockID) (transactionMetadata mempool.TransactionMetadata, exists bool)

	Account(accountID iotago.AccountID, targetIndex iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error)
	AddAccount(account *utxoledger.Output) error

	Output(id iotago.OutputID) (*utxoledger.Output, error)
	OutputOrSpent(id iotago.OutputID) (output *utxoledger.Output, spent *utxoledger.Spent, err error)
	ForEachUnspentOutput(func(output *utxoledger.Output) bool) error
	AddUnspentOutput(unspentOutput *utxoledger.Output) error
	StateDiffs(index iotago.SlotIndex) (*utxoledger.SlotDiff, error)

	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, BlockVoteRank]

	CommitSlot(index iotago.SlotIndex) (stateRoot, mutationRoot, accountRoot iotago.Identifier, err error)

	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error

	module.Interface
}
