package storage

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/utxoledger"
	"github.com/iotaledger/iota-core/pkg/storage/permanent"
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

// Ledger returns the ledger storage (or a specialized sub-storage if a realm is provided).
func (s *Storage) Ledger() *utxoledger.Manager {
	return s.permanent.UTXOLedger()
}
