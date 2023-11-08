package debugapi

import (
	"github.com/iotaledger/hive.go/ads"
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

func storeTransactionsPerSlot(scd *notarization.SlotCommittedDetails) error {
	slot := scd.Commitment.Slot()
	stateDiff, err := deps.Protocol.MainEngineInstance().Ledger.MemPool().StateDiff(slot)
	if err != nil {
		return ierrors.Wrapf(err, "failed to retrieve state diff for slot %d", slot)
	}
	mutationsTree := ads.NewSet[iotago.Identifier](
		mapdb.NewMapDB(),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		iotago.TransactionID.Bytes,
		iotago.TransactionIDFromBytes,
	)
	tcs := &TransactionsChangesResponse{
		Index:                slot,
		IncludedTransactions: make([]string, 0),
	}

	var innerErr error
	stateDiff.ExecutedTransactions().ForEach(func(_ iotago.TransactionID, txMeta mempool.TransactionMetadata) bool {
		tcs.IncludedTransactions = append(tcs.IncludedTransactions, txMeta.ID().String())
		if err := mutationsTree.Add(txMeta.ID()); err != nil {
			innerErr = ierrors.Wrapf(err, "failed to add transaction to mutations tree, txID: %s", txMeta.ID())

			return false
		}

		return true
	})

	tcs.MutationsRoot = mutationsTree.Root().String()

	transactionsPerSlot[slot] = tcs

	return innerErr
}

func getSlotTransactionIDs(slot iotago.SlotIndex) (*TransactionsChangesResponse, error) {
	if slotDiff, exists := transactionsPerSlot[slot]; exists {
		return slotDiff, nil
	}

	return nil, ierrors.Errorf("cannot find transaction storage bucket for slot %d", slot)
}
