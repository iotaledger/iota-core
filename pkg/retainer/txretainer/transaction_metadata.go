package txretainer

import (
	"encoding/hex"
	"fmt"

	iotago "github.com/iotaledger/iota.go/v4"
)

type txMetadata struct {
	TransactionID          []byte           `gorm:"primaryKey;unique;notnull"`
	ValidSignature         bool             `gorm:"notnull"`
	EarliestAttachmentSlot iotago.SlotIndex `gorm:"notnull;index:earliest_attachment_slots"`
	State                  byte             `gorm:"notnull"`
	FailureReason          byte             `gorm:"notnull"`
	ErrorMsg               *string
}

func (m *txMetadata) String() string {
	return fmt.Sprintf("tx metadata => TxID: %s, CreatedAtSlot: %d, ValidSignature: %t, State: %d, Error: %d, ErrorMsg: \"%s\"", hex.EncodeToString(m.TransactionID), m.EarliestAttachmentSlot, m.ValidSignature, m.State, m.FailureReason, *m.ErrorMsg)
}
