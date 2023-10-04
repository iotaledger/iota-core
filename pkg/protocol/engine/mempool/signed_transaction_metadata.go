package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type SignedTransactionMetadata interface {
	ID() iotago.SignedTransactionID

	SignedTransaction() SignedTransaction

	OnSignaturesValid(func()) (unsubscribe func())

	OnSignaturesInvalid(func(err error)) (unsubscribe func())

	TransactionMetadata() TransactionMetadata

	Attachments() []iotago.BlockID
}
