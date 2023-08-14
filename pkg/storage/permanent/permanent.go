package permanent

import (
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
)

const (
	settingsPrefix byte = iota
	commitmentsPrefix
	ledgerPrefix
	accountsPrefix
	latestNonEmptySlotPrefix
)

type Permanent struct {
	dbConfig     database.Config
	store        *database.DBInstance
	errorHandler func(error)

	settings    *Settings
	commitments *Commitments

	ledger             kvstore.KVStore
	accounts           kvstore.KVStore
	latestNonEmptySlot kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(dbConfig database.Config, errorHandler func(error), opts ...options.Option[Permanent]) *Permanent {
	return options.Apply(&Permanent{
		errorHandler: errorHandler,
		dbConfig:     dbConfig,
	}, opts, func(p *Permanent) {
		p.store = database.NewDBInstance(p.dbConfig)
		p.settings = NewSettings(lo.PanicOnErr(p.store.KVStore().WithExtendedRealm(kvstore.Realm{settingsPrefix})))
		p.commitments = NewCommitments(lo.PanicOnErr(p.store.KVStore().WithExtendedRealm(kvstore.Realm{commitmentsPrefix})), p.settings.APIProvider())
		p.ledger = lo.PanicOnErr(p.store.KVStore().WithExtendedRealm(kvstore.Realm{ledgerPrefix}))
		p.accounts = lo.PanicOnErr(p.store.KVStore().WithExtendedRealm(kvstore.Realm{accountsPrefix}))
		p.latestNonEmptySlot = lo.PanicOnErr(p.store.KVStore().WithExtendedRealm(kvstore.Realm{latestNonEmptySlotPrefix}))
	})
}

func (p *Permanent) Settings() *Settings {
	return p.settings
}

func (p *Permanent) Commitments() *Commitments {
	return p.commitments
}

// Accounts returns the Accounts storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Accounts(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.accounts
	}

	return lo.PanicOnErr(p.accounts.WithExtendedRealm(optRealm))
}

func (p *Permanent) LatestNonEmptySlot(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.latestNonEmptySlot
	}

	return lo.PanicOnErr(p.latestNonEmptySlot.WithExtendedRealm(optRealm))
}

// Ledger returns the ledger storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Ledger(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.ledger
	}

	return lo.PanicOnErr(p.ledger.WithExtendedRealm(optRealm))
}

// Size returns the size of the permanent storage.
func (p *Permanent) Size() int64 {
	dbSize, err := ioutils.FolderSize(p.dbConfig.Directory)
	if err != nil {
		p.errorHandler(ierrors.Wrapf(err, "dbDirectorySize failed for %s", p.dbConfig.Directory))
		return 0
	}

	return dbSize
}

func (p *Permanent) Shutdown() {
	p.store.Close()
}
