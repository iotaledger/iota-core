package mempool

import iotago "github.com/iotaledger/iota.go/v4"

type SignedTransactionMetadata interface {
	ID() iotago.SignedTransactionID

	SignedTransaction() SignedTransaction

	OnSignaturesValid(callback func()) (unsubscribe func())

	OnSignaturesInvalid(callback func(err error)) (unsubscribe func())

	SignaturesInvalid() error

	TransactionMetadata() TransactionMetadata

	Attachments() []iotago.BlockID
}
