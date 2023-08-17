package storage

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
	iotago "github.com/iotaledger/iota.go/v4"
)

func (s *Storage) Settings() *permanent.Settings {
	return s.permanent.Settings()
}

func (s *Storage) Commitments() *permanent.Commitments {
	return s.permanent.Commitments()
}

// Accounts returns the Accounts storage (or a specialized sub-storage if a realm is provided).
func (s *Storage) Accounts(optRealm ...byte) kvstore.KVStore {
	return s.permanent.Accounts(optRealm...)
}

func (s *Storage) LatestNonEmptySlot(optRealm ...byte) kvstore.KVStore {
	return s.permanent.LatestNonEmptySlot(optRealm...)
}

// Ledger returns the ledger storage (or a specialized sub-storage if a realm is provided).
func (s *Storage) Ledger(optRealm ...byte) kvstore.KVStore {
	return s.permanent.Ledger(optRealm...)
}

func (s *Storage) SetLedgerPruningFunc(ledgerPruningFunc func(epoch iotago.EpochIndex)) {
	s.ledgerPruningFunc = ledgerPruningFunc
}
