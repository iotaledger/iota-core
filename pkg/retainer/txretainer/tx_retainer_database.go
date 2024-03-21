package txretainer

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/iota-core/pkg/storage"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

var (
	dbTables = []interface{}{
		&TransactionMetadata{},
	}
)

// transactionRetainerDatabase is a wrapper around the gorm database to store transaction metadata.
type transactionRetainerDatabase struct {
	dbExecFunc storage.SQLDatabaseExecFunc
}

// NewTransactionRetainerDB creates a new transaction retainer database.
//
//nolint:revive // only used in the tests
func NewTransactionRetainerDB(dbExecFunc storage.SQLDatabaseExecFunc) *transactionRetainerDatabase {
	// create tables and indexes in the gorm DB if needed.
	// HINT: it is sufficient to call only once AutoMigrate here, because the underlying database won't be changed after the engine is started.
	// The transaction retainer has the same lifetime as the engine and so does the retainer database.
	if err := dbExecFunc(func(db *gorm.DB) error {
		return db.AutoMigrate(dbTables...)
	}); err != nil {
		panic(ierrors.Wrap(err, "failed to auto migrate tables"))
	}

	return &transactionRetainerDatabase{
		dbExecFunc: dbExecFunc,
	}
}

// insertTransactionMetadata inserts the given transaction metadata into the database.
func (r *transactionRetainerDatabase) insertTransactionMetadata(dbTx *gorm.DB, txMeta *TransactionMetadata) error {
	return dbTx.Create(txMeta).Error
}

// InsertTxMetadata inserts the given transaction metadata into the database.
func (r *transactionRetainerDatabase) InsertTxMetadata(newTxMeta *TransactionMetadata) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return r.insertTransactionMetadata(tx, newTxMeta)
		})
	}); err != nil {
		return ierrors.Wrap(err, "failed to update transaction metadata")
	}

	return nil
}

// ApplyTxMetadataChanges applies the given uncommittedTxMetadataChanges to the database in a single atomic transaction.
func (r *transactionRetainerDatabase) ApplyTxMetadataChanges(uncommittedTxMetadataChanges map[iotago.TransactionID]*TransactionMetadata) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			for _, newTxMeta := range uncommittedTxMetadataChanges {
				if err := r.insertTransactionMetadata(tx, newTxMeta); err != nil {
					return err
				}
			}

			return nil
		})
	}); err != nil {
		return ierrors.Wrap(err, "failed to apply transaction metadata changes")
	}

	return nil
}

// Rollback deletes all transaction metadata where the block slot of the earliest attachment is greater than the given slot.
// This is used to rollback the transaction metadata after chain switching.
func (r *transactionRetainerDatabase) Rollback(targetSlot iotago.SlotIndex) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		// delete all txMetadata where the block slot of the earliest attachment is greater than the given slot
		return db.Where("earliest_attachment_slot > ?", targetSlot).Delete(&TransactionMetadata{}).Error
	}); err != nil {
		return ierrors.Wrap(err, "failed to delete transaction metadata")
	}

	return nil
}

// Prune deletes all transaction metadata where the block slot of the earliest attachment is smaller or equal the given slot.
func (r *transactionRetainerDatabase) Prune(targetSlot iotago.SlotIndex) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		// delete all txMetadata where the block slot of the earliest attachment is smaller or equal the given slot
		return db.Where("earliest_attachment_slot <= ?", targetSlot).Delete(&TransactionMetadata{}).Error
	}); err != nil {
		return ierrors.Wrap(err, "failed to delete transaction metadata")
	}

	return nil
}

// TransactionMetadataByID returns the transaction metadata of a transaction by its ID.
// If no transaction metadata is found, nil is returned.
// There can be several entries for the same transaction ID in the database, but only one with the highest priority is returned.
// The priority is determined by the following conditions:
//   - txMeta with "accepted state" has the highest priority
//   - then txMeta with valid signature (True over False)
//   - then txMeta with the earliest attachment slot (descending order)
//   - then everything else
func (r *transactionRetainerDatabase) TransactionMetadataByID(transactionID iotago.TransactionID) (*TransactionMetadata, error) {
	txMeta := &TransactionMetadata{}

	if err := r.dbExecFunc(func(dbTx *gorm.DB) error {
		return dbTx.Where("transaction_id = ?", transactionID[:]).
			Order(fmt.Sprintf("state = %d DESC, valid_signature DESC, earliest_attachment_slot DESC", api.TransactionStateAccepted)).
			Take(txMeta).Error
	}); err != nil {
		if !ierrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ierrors.Wrapf(err, "failed to query transaction metadata for transaction ID %s", transactionID)
		}

		// no entry found
		//nolint:nilnil // we want to return nil here
		return nil, nil
	}

	return txMeta, nil
}
