package ledger

import (
	"io"

	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts/mana"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool/conflictdag"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Ledger interface {
	AttachTransaction(block *blocks.Block) (transactionMetadata mempool.TransactionMetadata, containsTransaction bool)
	OnTransactionAttached(callback func(transactionMetadata mempool.TransactionMetadata), opts ...event.Option)
	Output(id iotago.IndexedUTXOReferencer) (*utxoledger.Output, error)
	Account(accountID iotago.AccountID, optTargetIndex ...iotago.SlotIndex) (accountData *accounts.AccountData, exists bool, err error)
	IsOutputUnspent(outputID iotago.OutputID) (bool, error)
	Spent(outputID iotago.OutputID) (*utxoledger.Spent, error)
	TransactionMetadata(id iotago.TransactionID) (transactionMetadata mempool.TransactionMetadata, exists bool)
	TransactionMetadataByAttachment(blockID iotago.BlockID) (transactionMetadata mempool.TransactionMetadata, exists bool)
	CommitSlot(index iotago.SlotIndex) (stateRoot, mutationRoot, accountRoot iotago.Identifier, err error)
	ConflictDAG() conflictdag.ConflictDAG[iotago.TransactionID, iotago.OutputID, BlockVotePower]
	StateDiffs(index iotago.SlotIndex) (*utxoledger.SlotDiff, error)
	AddUnspentOutput(unspentOutput *utxoledger.Output) error
	ManaManager() *mana.Manager
	AddAccount(account *utxoledger.Output) error
	Import(reader io.ReadSeeker) error
	Export(writer io.WriteSeeker, targetIndex iotago.SlotIndex) error
	IsBlockIssuerAllowed(*iotago.Block) bool

	module.Interface
}
