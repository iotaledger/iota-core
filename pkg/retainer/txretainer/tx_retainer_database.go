package txretainer

import (
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

// updateTransactionMetadata updates the transaction metadata for the given txID in the given database transaction.
func (r *transactionRetainerDatabase) updateTransactionMetadata(dbTx *gorm.DB, newTxMeta *TransactionMetadata) error {
	txMeta := &TransactionMetadata{}
	entryFound := false

	txID := newTxMeta.TransactionID

	// check if a txMeta with accepted state for the given txID already exists.
	// if yes, we don't store new txMetas at all.
	if err := dbTx.First(txMeta, &TransactionMetadata{TransactionID: txID, ValidSignature: true, State: byte(api.TransactionStateAccepted)}).Error; err == nil {
		// a txMeta with accepted state for the given txID already exists, this is the final state of a txMeta and should not be overwritten.
		return nil
	} else if !ierrors.Is(err, gorm.ErrRecordNotFound) {
		return ierrors.Wrapf(err, "failed to check if an accepted transaction metadata with valid signature exists for txID %s", iotago.TransactionID(txID).ToHex())
	}

	// check if a signed txMeta for the given txID already exists.
	// if yes, we only store new txMetas with a valid signature as well,
	// otherwise an attacker could overwrite valid txMeta with invalid txMeta.
	if err := dbTx.First(txMeta, &TransactionMetadata{TransactionID: txID, ValidSignature: true}).Error; err == nil {
		// a signed txMeta for the given txID already exists, we only store new txMetas with a valid signature as well.
		if !newTxMeta.ValidSignature {
			return nil
		}
		entryFound = true
	} else if !ierrors.Is(err, gorm.ErrRecordNotFound) {
		return ierrors.Wrapf(err, "failed to check if a transaction metadata with valid signature exists for txID %s", iotago.TransactionID(txID).ToHex())
	}

	if entryFound {
		// an existing txMeta was found, we need to update it.
		txMeta.TransactionID = newTxMeta.TransactionID
		txMeta.ValidSignature = newTxMeta.ValidSignature
		txMeta.EarliestAttachmentSlot = newTxMeta.EarliestAttachmentSlot
		txMeta.State = newTxMeta.State
		txMeta.FailureReason = newTxMeta.FailureReason
		txMeta.ErrorMsg = newTxMeta.ErrorMsg
	} else {
		// no existing txMeta was found, we need to create a new one.
		txMeta = newTxMeta
	}

	// create or update the txMeta
	return dbTx.Save(txMeta).Error
}

// UpdateTxMetadata updates the transaction metadata for the given txID.
func (r *transactionRetainerDatabase) UpdateTxMetadata(newTxMeta *TransactionMetadata) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			return r.updateTransactionMetadata(tx, newTxMeta)
		})
	}); err != nil {
		return ierrors.Wrap(err, "failed to update transaction metadata")
	}

	return nil
}

// ApplyTxMetadataChanges applies the given uncommitedTxMetadataChanges to the database in a single atomic transaction.
func (r *transactionRetainerDatabase) ApplyTxMetadataChanges(uncommitedTxMetadataChanges map[iotago.TransactionID]*TransactionMetadata) error {
	if err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			for _, newTxMeta := range uncommitedTxMetadataChanges {
				if err := r.updateTransactionMetadata(tx, newTxMeta); err != nil {
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

func (r *transactionRetainerDatabase) TransactionMetadataByID(transactionID iotago.TransactionID) (*TransactionMetadata, error) {
	txMeta := &TransactionMetadata{}

	if err := r.dbExecFunc(func(dbTx *gorm.DB) error {
		// txMeta with accepted state has the highest priority
		if err := dbTx.First(txMeta, &TransactionMetadata{TransactionID: transactionID[:], ValidSignature: true, State: byte(api.TransactionStateAccepted)}).Error; err == nil {
			// entry found
			return nil
		} else if !ierrors.Is(err, gorm.ErrRecordNotFound) {
			return ierrors.Wrapf(err, "failed to check if an accepted transaction metadata with valid signature exists for txID %s", transactionID.ToHex())
		}

		// then txMeta with valid signature
		if err := dbTx.First(txMeta, &TransactionMetadata{TransactionID: transactionID[:], ValidSignature: true}).Error; err == nil {
			// entry found
			return nil
		} else if !ierrors.Is(err, gorm.ErrRecordNotFound) {
			return ierrors.Wrapf(err, "failed to check if a transaction metadata with valid signature exists for txID %s", transactionID.ToHex())
		}

		// then everything else
		return dbTx.First(txMeta, &TransactionMetadata{TransactionID: transactionID[:]}).Error
	}); err != nil {
		if !ierrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ierrors.Wrap(err, "failed to query transaction metadata")
		}

		// no entry found
		//nolint:nilnil // we want to return nil here
		return nil, nil
	}

	return txMeta, nil
}
