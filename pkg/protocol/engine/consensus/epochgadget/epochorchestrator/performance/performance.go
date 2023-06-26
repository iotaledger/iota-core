package performance

import (
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/advancedset"
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

func (m *Manager) RewardsRoot(epochIndex iotago.EpochIndex) iotago.Identifier {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	return iotago.Identifier(ads.NewMap[iotago.AccountID, AccountRewards](m.rewardsStorage(epochIndex)).Root())
}

func (m *Manager) RegisterCommittee(epochIndex iotago.EpochIndex, committee *account.Accounts) error {
	return m.storeCommitteeForEpoch(epochIndex, committee)
}

func (m *Manager) ApplyEpoch(epochIndex iotago.EpochIndex) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	accounts := m.loadCommitteeForEpoch(epoch)

	epochSlotStart := m.timeProvider.EpochStart(epochIndex)
	epochSlotEnd := m.timeProvider.EpochEnd(epochIndex)

	profitMargin := calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake())
	poolsStats := PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := m.poolStatsStore.Set(epochIndex.Bytes(), lo.PanicOnErr(poolsStats.Bytes())); err != nil {
		panic(errors.Wrapf(err, "failed to store pool stats for epoch %d", epochIndex))
	}

	committee.ForEach(func(id iotago.AccountID, pool *account.Pool) bool {
		intermediateFactors := make([]uint64, 0)
		for slot := epochSlotStart; slot <= epochSlotEnd; slot++ {
			performanceFactorStorage := m.performanceFactorsFunc(slot)
			if performanceFactorStorage == nil {
				intermediateFactors = append(intermediateFactors, 0)
			}

			pf, err := performanceFactorStorage.Load(id)
			if err != nil {
				panic(errors.Wrapf(err, "failed to load performance factor for account %s", id))
				return false
			}

			intermediateFactors = append(intermediateFactors, pf)

		}

		rewardsTree.Set(id, &RewardsForAccount{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward(committee.TotalValidatorStake(), committee.TotalStake(), profitMargin, pool.FixedCost, aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		})

		return true
	})
}

func (m *Tracker) poolStats(epoch iotago.EpochIndex) (poolStats *PoolsStats, err error) {
	poolStats = new(PoolsStats)
	poolStatsBytes, err := m.poolStatsStore.Get(epochIndex.Bytes())
	if err != nil {
		return poolStats, errors.Wrapf(err, "failed to get pool stats for epoch %d", epochIndex)
	}

	if _, err := poolStats.FromBytes(poolStatsBytes); err != nil {
		return poolStats, errors.Wrapf(err, "failed to parse pool stats for epoch %d", epochIndex)
	}

	return poolStats, nil
}

func (m *Tracker) loadCommitteeForEpoch(epoch iotago.EpochIndex) *account.Accounts {
	accountsBytes, err := m.committeeStore.Get(epoch.Bytes())
	if err != nil {
		panic(errors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	committee, err = account.AccountsFromBytes(accountsBytes)
	if err != nil {
		panic(errors.Wrapf(err, "failed to parse committee for epoch %d", epochIndex))
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
