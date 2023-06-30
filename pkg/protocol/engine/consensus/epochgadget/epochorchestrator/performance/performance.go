package performance

import (
	"sync"

	"github.com/cockroachdb/errors"

	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Tracker struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  *kvstore.TypedStore[iotago.EpochIndex, *PoolsStats]
	committeeStore  *kvstore.TypedStore[iotago.EpochIndex, *account.Accounts]

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
		rewardBaseStore: rewardsBaseStore,
		poolStatsStore: kvstore.NewTypedStore(poolStatsStore,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
			(*PoolsStats).Bytes,
			PoolsStatsFromBytes,
		),
		committeeStore: kvstore.NewTypedStore(committeeStore,
			iotago.EpochIndex.Bytes,
			iotago.EpochIndexFromBytes,
			(*account.Accounts).Bytes,
			account.AccountsFromBytes,
		),
		performanceFactorsFunc: performanceFactorsFunc,
		timeProvider:           timeProvider,
		decayProvider:          decayProvider,
	}
}

func (m *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return m.committeeStore.Set(epoch, committee)
}

func (m *Tracker) BlockAccepted(block *blocks.Block) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	m.performanceFactorsMutex.Lock()
	defer m.performanceFactorsMutex.Unlock()

	// TODO: check if this block is a validator block

	performanceFactors := m.performanceFactorsFunc(block.ID().Index())
	pf, err := performanceFactors.Load(block.ProtocolBlock().IssuerID)
	if err != nil {
		panic(err)
	}

	err = performanceFactors.Store(block.ProtocolBlock().IssuerID, pf+1)
	if err != nil {
		panic(err)
	}
}

func (m *Tracker) ApplyEpoch(epoch iotago.EpochIndex, committee *account.Accounts) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	epochSlotStart := m.timeProvider.EpochStart(epoch)
	epochSlotEnd := m.timeProvider.EpochEnd(epoch)

	profitMargin := calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake())
	poolsStats := &PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := m.poolStatsStore.Set(epoch, poolsStats); err != nil {
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

		rewardsMap := ads.NewMap[iotago.AccountID, *PoolRewards](m.rewardsStorage(epoch),
			iotago.Identifier.Bytes,
			iotago.IdentifierFromBytes,
			(*PoolRewards).Bytes,
			PoolRewardsFromBytes,
		)

		rewardsMap.Set(accountID, &PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: poolReward(epochSlotEnd, committee.TotalValidatorStake(), committee.TotalStake(), pool.PoolStake, pool.ValidatorStake, pool.FixedCost, aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		})

		return true
	})
}

func (m *Tracker) EligibleValidatorCandidates(epoch iotago.EpochIndex) *advancedset.AdvancedSet[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for

	return &advancedset.AdvancedSet[iotago.AccountID]{}
}

func (m *Tracker) LoadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	c, err := m.committeeStore.Get(epoch)
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, false
		}
		panic(errors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}
	return c, true
}

func aggregatePerformanceFactors(pfs []uint64) uint64 {
	var sum uint64
	for _, pf := range pfs {
		sum += pf
	}

	return sum / uint64(len(pfs))
}
