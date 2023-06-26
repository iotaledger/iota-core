package performance

import (
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  kvstore.KVStore
	committeeStore  kvstore.KVStore

	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors

	timeProvider *iotago.TimeProvider

	decayProvider *iotago.ManaDecayProvider

	performanceFactorsMutex sync.RWMutex
	mutex                   sync.RWMutex
}

func NewTracker(
	rewardsBaseStore kvstore.KVStore,
	poolStatsStore kvstore.KVStore,
	committeeStore kvstore.KVStore,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors,
	timeProvider *iotago.TimeProvider,
	decayProvider *iotago.ManaDecayProvider,
) *Tracker {
	return &Tracker{
		rewardBaseStore:        rewardsBaseStore,
		poolStatsStore:         poolStatsStore,
		committeeStore:         committeeStore,
		performanceFactorsFunc: performanceFactorsFunc,
		timeProvider:           timeProvider,
		decayProvider:          decayProvider,
	}
}

func (m *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return m.storeCommitteeForEpoch(epoch, committee)
}

func (m *Tracker) BlockAccepted(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.performanceFactorsMutex.Lock()
	defer m.performanceFactorsMutex.Unlock()

	// TODO: check if this block is a validator block

	performanceFactors := m.performanceFactorsFunc(block.ID().Index())
	pf, err := performanceFactors.Load(block.Block().IssuerID)
	if err != nil {
		panic(err)
	}

	err = performanceFactors.Store(block.Block().IssuerID, pf+1)
	if err != nil {
		panic(err)
	}
}

func (m *Tracker) ApplyEpoch(epoch iotago.EpochIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	committee, exists := m.loadCommitteeForEpoch(epoch)
	if !exists {
		// If the committee for the epoch wasn't set before we promote the current one.
		committee, exists = m.loadCommitteeForEpoch(epoch - 1)
		if !exists {
			panic("committee for epoch not found")
		}
		m.RegisterCommittee(epoch, committee)
	}

	epochSlotStart := m.timeProvider.EpochStart(epoch)
	epochSlotEnd := m.timeProvider.EpochEnd(epoch)

	profitMargin := calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake())
	poolsStats := PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := m.poolStatsStore.Set(epoch.Bytes(), lo.PanicOnErr(poolsStats.Bytes())); err != nil {
		panic(errors.Wrapf(err, "failed to store pool stats for epoch %d", epoch))
	}

	committee.ForEach(func(accountID iotago.AccountID, pool *account.Pool) bool {
		intermediateFactors := make([]uint64, 0)
		for slot := epochSlotStart; slot <= epochSlotEnd; slot++ {
			performanceFactorStorage := m.performanceFactorsFunc(slot)
			if performanceFactorStorage == nil {
				intermediateFactors = append(intermediateFactors, 0)
			}

			pf, err := performanceFactorStorage.Load(accountID)
			if err != nil {
				panic(errors.Wrapf(err, "failed to load performance factor for account %s", accountID))
				return false
			}

			intermediateFactors = append(intermediateFactors, pf)

		}

		rewardsTree := ads.NewMap[iotago.AccountID, PoolRewards](m.rewardsStorage(epoch))

		rewardsTree.Set(accountID, &PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward(committee.TotalValidatorStake(), committee.TotalStake(), profitMargin, pool.FixedCost, aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		})

		return true
	})
}

func (m *Tracker) poolStats(epoch iotago.EpochIndex) (poolStats *PoolsStats, err error) {
	poolStats = new(PoolsStats)
	poolStatsBytes, err := m.poolStatsStore.Get(epoch.Bytes())
	if err != nil {
		return poolStats, errors.Wrapf(err, "failed to get pool stats for epoch %d", epoch)
	}

	if _, err := poolStats.FromBytes(poolStatsBytes); err != nil {
		return poolStats, errors.Wrapf(err, "failed to parse pool stats for epoch %d", epoch)
	}

	return poolStats, nil
}

func (m *Tracker) loadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	accountsBytes, err := m.committeeStore.Get(epoch.Bytes())
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, false
		}
		panic(errors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	committee, err = account.AccountsFromBytes(accountsBytes)
	if err != nil {
		panic(errors.Wrapf(err, "failed to parse committee for epoch %d", epoch))
	}

	return committee, true
}

func (m *Tracker) storeCommitteeForEpoch(epochIndex iotago.EpochIndex, committee *account.Accounts) error {
	committeeBytes, err := committee.Bytes()
	if err != nil {
		return err
	}

	if err := m.committeeStore.Set(epochIndex.Bytes(), committeeBytes); err != nil {
		return err
	}

	return nil
}

func aggregatePerformanceFactors(pfs []uint64) uint64 {
	var sum uint64
	for _, pf := range pfs {
		sum += pf
	}

	return sum / uint64(len(pfs))
}
