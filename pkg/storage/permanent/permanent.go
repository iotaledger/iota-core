package permanent

import (
	"github.com/pkg/errors"

	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/ioutils"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/iota-core/pkg/storage/database"
	"github.com/iotaledger/iota-core/pkg/storage/utils"
)

const (
	settingsPrefix byte = iota
	sybilProtectionPrefix
	attestationsPrefix
	ledgerPrefix
	accountsPrefix
	rewardsPrefix
	poolStatsPrefix
	committeePrefix
)

type Permanent struct {
	dbConfig      database.Config
	store         kvstore.KVStore
	healthTracker *kvstore.StoreHealthTracker
	errorHandler  func(error)

	settings    *Settings
	commitments *Commitments

	sybilProtection kvstore.KVStore
	attestations    kvstore.KVStore
	ledger          kvstore.KVStore
	accounts        kvstore.KVStore
	rewards         kvstore.KVStore
	poolStats       kvstore.KVStore
	committee       kvstore.KVStore
}

// New returns a new permanent storage instance.
func New(baseDir *utils.Directory, dbConfig database.Config, errorHandler func(error), opts ...options.Option[Permanent]) *Permanent {
	return options.Apply(&Permanent{
		errorHandler: errorHandler,
	}, opts, func(p *Permanent) {

		var err error
		p.store, err = database.StoreWithDefaultSettings(dbConfig.Directory, true, dbConfig.Engine)
		if err != nil {
			panic(err)
		}

		p.healthTracker, err = kvstore.NewStoreHealthTracker(p.store, dbConfig.PrefixHealth, dbConfig.Version, nil)
		if err != nil {
			panic(errors.Wrapf(err, "database in %s is corrupted, delete database and resync node", dbConfig.Directory))
		}
		if err = p.healthTracker.MarkCorrupted(); err != nil {
			panic(err)
		}

		p.settings = NewSettings(lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{settingsPrefix})))
		p.commitments = NewCommitments(baseDir.Path("commitments.bin"), p.settings.APIForSlot)
		p.sybilProtection = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{sybilProtectionPrefix}))
		p.attestations = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{attestationsPrefix}))
		p.ledger = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{ledgerPrefix}))
		p.accounts = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{accountsPrefix}))
		p.rewards = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{rewardsPrefix}))
		p.poolStats = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{poolStatsPrefix}))
		p.committee = lo.PanicOnErr(p.store.WithExtendedRealm(kvstore.Realm{committeePrefix}))
	})
}

func (p *Permanent) Settings() *Settings {
	return p.settings
}

func (p *Permanent) Commitments() *Commitments {
	return p.commitments
}

// SybilProtection returns the sybil protection storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) SybilProtection(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.sybilProtection
	}

	return lo.PanicOnErr(p.sybilProtection.WithExtendedRealm(optRealm))
}

// Accounts returns the Accounts storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Accounts(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.accounts
	}

	return lo.PanicOnErr(p.accounts.WithExtendedRealm(optRealm))
}

// TODO: Rewards and PoolStats should be pruned after one year, so they are not really permanent.

// Rewards returns the Rewards storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Rewards(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.rewards
	}

	return lo.PanicOnErr(p.rewards.WithExtendedRealm(optRealm))
}

// PoolStats returns the PoolStats storage.
func (p *Permanent) PoolStats(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.poolStats
	}

	return lo.PanicOnErr(p.poolStats.WithExtendedRealm(optRealm))
}

func (p *Permanent) Committee(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.committee
	}

	return lo.PanicOnErr(p.committee.WithExtendedRealm(optRealm))
}

// Attestations returns the "attestations" storage (or a specialized sub-storage if a realm is provided).
func (p *Permanent) Attestations(optRealm ...byte) kvstore.KVStore {
	if len(optRealm) == 0 {
		return p.attestations
	}

	return lo.PanicOnErr(p.attestations.WithExtendedRealm(optRealm))
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
		p.errorHandler(errors.Wrapf(err, "dbDirectorySize failed for %s", p.dbConfig.Directory))
		return 0
	}

	return dbSize
}

func (p *Permanent) Shutdown() {
	if err := p.commitments.Close(); err != nil {
		panic(err)
	}

	if err := p.healthTracker.MarkHealthy(); err != nil {
		panic(err)
	}
	if err := database.FlushAndClose(p.store); err != nil {
		panic(err)
	}
}
