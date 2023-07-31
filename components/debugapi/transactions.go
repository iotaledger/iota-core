package debugapi

import (
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

var transactionsPerSlot map[iotago.SlotIndex]*TransactionsChangesResponse

func init() {
	transactionsPerSlot = make(map[iotago.SlotIndex]*TransactionsChangesResponse)
}

func storeTransactionsPerSlot(scd *notarization.SlotCommittedDetails) {
	slot := scd.Commitment.Index()
	stateDiff := deps.Protocol.MainEngineInstance().Ledger.MemPool().StateDiff(slot)
	mutationsTree := ds.NewAuthenticatedSet(mapdb.NewMapDB(), iotago.Identifier.Bytes, iotago.IdentifierFromBytes)
	tcs := &TransactionsChangesResponse{
		Index:                slot,
		IncludedTransactions: make([]string, 0),
	}

	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.Identifier, txMeta mempool.TransactionMetadata) bool {
		tcs.IncludedTransactions = append(tcs.IncludedTransactions, txMeta.ID().String())
		mutationsTree.Add(txMeta.ID())

		return true
	})

	tcs.MutationsRoot = iotago.Identifier(mutationsTree.Root()).String()

	transactionsPerSlot[slot] = tcs
}

func getSlotTransactionIDs(slot iotago.SlotIndex) (*TransactionsChangesResponse, error) {
	if slotDiff, exists := transactionsPerSlot[slot]; exists {
		return slotDiff, nil
	}

	return nil, ierrors.Errorf("cannot find transaction storage bucket for slot %d", slot)
}
