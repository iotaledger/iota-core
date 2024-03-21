package debugapi

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore/mapdb"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/notarization"
	iotago "github.com/iotaledger/iota.go/v4"
)

var transactionsPerSlot map[iotago.SlotIndex]*TransactionsChangesResponse

func init() {
	transactionsPerSlot = make(map[iotago.SlotIndex]*TransactionsChangesResponse)
}

func storeTransactionsPerSlot(scd *notarization.SlotCommittedDetails) error {
	slot := scd.Commitment.Slot()

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

	for _, transaction := range scd.Mutations {
		txID, err := transaction.ID()
		if err != nil {
			return ierrors.Wrapf(err, "failed to calculate transactionID")
		}

		tcs.IncludedTransactions = append(tcs.IncludedTransactions, txID.String())
		if err = mutationsTree.Add(txID); err != nil {
			return ierrors.Wrapf(err, "failed to add transaction to mutations tree, txID: %s", txID)
		}
	}

	tcs.MutationsRoot = mutationsTree.Root().String()

	transactionsPerSlot[slot] = tcs

	return nil
}

func getSlotTransactionIDs(slot iotago.SlotIndex) (*TransactionsChangesResponse, error) {
	if slotDiff, exists := transactionsPerSlot[slot]; exists {
		return slotDiff, nil
	}

	return nil, ierrors.Errorf("cannot find transaction storage bucket for slot %d", slot)
}
