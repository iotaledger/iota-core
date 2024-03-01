package txretainer

import (
	"gorm.io/gorm"

	"github.com/iotaledger/hive.go/ierrors"
	iotago "github.com/iotaledger/iota.go/v4"
)

var (
	ErrEntryNotFound = ierrors.New("entry not found")

	dbTables = append([]interface{}{
		&Status{},
	}, metadataTables...)

	metadataTables = []interface{}{
		&txMetadata{},
	}
)

func (r *TransactionRetainer) handleError(err error, message string) error {
	if err != nil {
		err = ierrors.Wrap(err, message)
		r.errorHandler(err)
	}

	return err
}

func (r *TransactionRetainer) IsInitialized() bool {
	hasTable := false

	err := r.dbExecFunc(func(db *gorm.DB) error {
		hasTable = db.Migrator().HasTable(&Status{})
		return nil
	})
	r.handleError(err, "failed to check if store is initialized")

	return hasTable
}

func (r *TransactionRetainer) CreateTables() error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Migrator().CreateTable(dbTables...)
	})

	return r.handleError(err, "failed to create tables")
}

func (r *TransactionRetainer) AutoMigrate() error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		// Create the tables and indexes if needed
		return db.AutoMigrate(dbTables...)
	})

	return r.handleError(err, "failed to auto migrate tables")
}

func (r *TransactionRetainer) Status() (*Status, error) {
	status := &Status{}

	if err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Take(&status).Error
	}); err != nil {
		if ierrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEntryNotFound
		}

		return nil, r.handleError(err, "failed to get status")
	}

	return status, nil
}

func (r *TransactionRetainer) Clear() error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		// drop all tables
		if err := db.Migrator().DropTable(dbTables...); err != nil {
			return err
		}

		// re-create tables
		return db.Migrator().CreateTable(dbTables...)
	})

	return r.handleError(err, "failed to clear database")
}

func (r *TransactionRetainer) CloseDatabase() error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}

		return sqlDB.Close()
	})

	return r.handleError(err, "failed to close database")
}

func (r *TransactionRetainer) updateTxMetadata(txID iotago.TransactionID, newTxMeta *txMetadata) {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			txMeta := &txMetadata{}

			// init the txMeta if it does not exist yet, or get the existing one from the database
			tx.FirstOrInit(txMeta, &txMetadata{TransactionID: txID[:]})

			if txMeta.ValidSignature && !newTxMeta.ValidSignature {
				// do not update the entry in the database if the signature was valid before and is invalid now
				return nil
			}

			// reset former information if the new signature is valid and the former one was invalid
			if !txMeta.ValidSignature && newTxMeta.ValidSignature {
				txMeta.ValidSignature = true
				txMeta.EarliestAttachmentSlot = 0
				txMeta.State = 0
				txMeta.FailureReason = 0
				txMeta.ErrorMsg = nil
			}

			// if the new earliest attachment slot is set, update the field if it wasn't set before or it is smaller now
			if (newTxMeta.EarliestAttachmentSlot != 0) && ((txMeta.EarliestAttachmentSlot == 0) || (newTxMeta.EarliestAttachmentSlot < txMeta.EarliestAttachmentSlot)) {
				txMeta.EarliestAttachmentSlot = newTxMeta.EarliestAttachmentSlot
			}

			// update the state and failure reason
			txMeta.State = newTxMeta.State
			txMeta.FailureReason = newTxMeta.FailureReason

			if r.storeDebugErrorMessages {
				txMeta.ErrorMsg = newTxMeta.ErrorMsg
			}

			// create or update the txMeta
			return tx.Save(txMeta).Error
		})
	})

	r.handleError(err, "failed to update transaction metadata")
}

// used for chain switching to get a clean state of the newly forked database.
func (r *TransactionRetainer) deleteTxMetadataWithSlotGreaterThan(slot iotago.SlotIndex) error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		// delete all txMetadata where the block slot of the earliest attachment is greater than the given slot
		return db.Where("earliest_attachment_slot > ?", slot).Delete(&txMetadata{}).Error
	})

	return r.handleError(err, "failed to delete transaction metadata")
}

// used for pruning to get rid of old entries.
func (r *TransactionRetainer) deleteTxMetadataWithSlotSmallerOrEqualThan(slot iotago.SlotIndex) error {
	err := r.dbExecFunc(func(db *gorm.DB) error {
		// delete all txMetadata where the block slot of the earliest attachment is smaller or equal the given slot
		return db.Where("earliest_attachment_slot <= ?", slot).Delete(&txMetadata{}).Error
	})

	return r.handleError(err, "failed to delete transaction metadata")
}

func (r *TransactionRetainer) transactionMetadataByID(transactionID iotago.TransactionID) *txMetadata {
	var results []*txMetadata

	err := r.dbExecFunc(func(db *gorm.DB) error {
		return db.Model(&txMetadata{}).
			Where("transaction_id = ?", transactionID[:]).
			Limit(1).Find(&results).Error
	})

	r.handleError(err, "failed to query transaction metadata")

	if len(results) == 0 {
		return nil
	}

	return results[0]
}
