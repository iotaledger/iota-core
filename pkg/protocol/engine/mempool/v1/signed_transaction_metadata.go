package mempoolv1

import (
	"github.com/iotaledger/hive.go/ds/reactive"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/mempool"
	iotago "github.com/iotaledger/iota.go/v4"
)

type SignedTransactionMetadata struct {
	id                  iotago.SignedTransactionID
	signedTransaction   mempool.SignedTransaction
	transactionMetadata *TransactionMetadata
	attachments         reactive.Set[iotago.BlockID]
	attachmentsMutex    syncutils.RWMutex
	signaturesInvalid   reactive.Variable[error]
	signaturesValid     reactive.Event
	evicted             reactive.Event
}

func NewSignedTransactionMetadata(signedTransaction mempool.SignedTransaction, transactionMetadata *TransactionMetadata) *SignedTransactionMetadata {
	signedID := signedTransaction.MustID()

	return &SignedTransactionMetadata{
		id:                  signedID,
		signedTransaction:   signedTransaction,
		transactionMetadata: transactionMetadata,
		attachments:         reactive.NewSet[iotago.BlockID](),
		signaturesInvalid:   reactive.NewVariable[error](),
		signaturesValid:     reactive.NewEvent(),
		evicted:             reactive.NewEvent(),
	}
}

func (s *SignedTransactionMetadata) ID() iotago.SignedTransactionID {
	return s.id
}

func (s *SignedTransactionMetadata) SignedTransaction() mempool.SignedTransaction {
	return s.signedTransaction
}

func (s *SignedTransactionMetadata) TransactionMetadata() mempool.TransactionMetadata {
	return s.transactionMetadata
}

func (s *SignedTransactionMetadata) OnSignaturesInvalid(callback func(error)) (unsubscribe func()) {
	return s.signaturesInvalid.OnUpdate(func(_ error, err error) {
		callback(err)
	})
}

func (s *SignedTransactionMetadata) SignaturesInvalid() error {
	return s.signaturesInvalid.Get()
}

func (s *SignedTransactionMetadata) OnSignaturesValid(callback func()) (unsubscribe func()) {
	return s.signaturesValid.OnTrigger(callback)
}

func (s *SignedTransactionMetadata) IsEvicted() bool {
	return s.evicted.WasTriggered()
}

func (s *SignedTransactionMetadata) OnEvicted(callback func()) {
	s.evicted.OnTrigger(callback)
}

func (s *SignedTransactionMetadata) setEvicted() {
	s.evicted.Trigger()
}

func (s *SignedTransactionMetadata) Attachments() []iotago.BlockID {
	return s.attachments.ToSlice()
}

func (s *SignedTransactionMetadata) addAttachment(blockID iotago.BlockID) (added bool) {
	s.attachmentsMutex.Lock()
	defer s.attachmentsMutex.Unlock()

	return s.attachments.Add(blockID)
}

func (s *SignedTransactionMetadata) evictAttachment(id iotago.BlockID) {
	s.attachmentsMutex.Lock()
	defer s.attachmentsMutex.Unlock()

	if s.attachments.Delete(id) && s.attachments.IsEmpty() {
		s.setEvicted()
	}
}
