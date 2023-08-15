package performance

import (
	"github.com/iotaledger/hive.go/ads"
	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/storage/prunable"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
)

type Tracker struct {
	rewardBaseStore kvstore.KVStore
	poolStatsStore  *kvstore.TypedStore[iotago.EpochIndex, *PoolsStats]
	committeeStore  *kvstore.TypedStore[iotago.EpochIndex, *account.Accounts]

	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors
	latestAppliedEpoch     iotago.EpochIndex

	apiProvider api.Provider

	performanceFactorsMutex syncutils.RWMutex
	mutex                   syncutils.RWMutex
}

func NewTracker(
	rewardsBaseStore kvstore.KVStore,
	poolStatsStore kvstore.KVStore,
	committeeStore kvstore.KVStore,
	performanceFactorsFunc func(slot iotago.SlotIndex) *prunable.PerformanceFactors,
	latestAppliedEpoch iotago.EpochIndex,
	apiProvider api.Provider,
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
		latestAppliedEpoch:     latestAppliedEpoch,
		apiProvider:            apiProvider,
	}
}

func (t *Tracker) RegisterCommittee(epoch iotago.EpochIndex, committee *account.Accounts) error {
	return t.committeeStore.Set(epoch, committee)
}

func (t *Tracker) TrackValidationBlock(block *blocks.Block) {
	validatorBlock, isValidationBlock := block.ValidationBlock()
	if !isValidationBlock {
		return
	}

	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.performanceFactorsMutex.Lock()
	defer t.performanceFactorsMutex.Unlock()

	performanceFactors := t.performanceFactorsFunc(block.ID().Index())
	pf, err := performanceFactors.Load(block.ProtocolBlock().IssuerID)
	if err != nil {
		// TODO replace panic with errors in the future, like triggering an error event
		panic(ierrors.Errorf("failed to load performance factor for account %s", block.ProtocolBlock().IssuerID))
	}

	// TODO: store highest supported version per validator?
	_ = validatorBlock.HighestSupportedVersion

	err = performanceFactors.Store(block.ProtocolBlock().IssuerID, pf+1)
	if err != nil {
		// TODO replace panic with errors in the future, like triggering an error event
		panic(ierrors.Errorf("failed to store performance factor for account %s", block.ProtocolBlock().IssuerID))
	}
}

func (t *Tracker) ApplyEpoch(epoch iotago.EpochIndex, committee *account.Accounts) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	timeProvider := t.apiProvider.APIForEpoch(epoch).TimeProvider()
	epochStartSlot := timeProvider.EpochStart(epoch)
	epochEndSlot := timeProvider.EpochEnd(epoch)

	profitMargin := calculateProfitMargin(committee.TotalValidatorStake(), committee.TotalStake())
	poolsStats := &PoolsStats{
		TotalStake:          committee.TotalStake(),
		TotalValidatorStake: committee.TotalValidatorStake(),
		ProfitMargin:        profitMargin,
	}

	if err := t.poolStatsStore.Set(epoch, poolsStats); err != nil {
		panic(ierrors.Wrapf(err, "failed to store pool stats for epoch %d", epoch))
	}

	rewardsTree := ads.NewMap(t.rewardsStorage(epoch),
		iotago.Identifier.Bytes,
		iotago.IdentifierFromBytes,
		(*PoolRewards).Bytes,
		PoolRewardsFromBytes,
	)

	committee.ForEach(func(accountID iotago.AccountID, pool *account.Pool) bool {
		intermediateFactors := make([]uint64, 0, epochEndSlot+1-epochStartSlot)
		for slot := epochStartSlot; slot <= epochEndSlot; slot++ {
			performanceFactorStorage := t.performanceFactorsFunc(slot)
			if performanceFactorStorage == nil {
				intermediateFactors = append(intermediateFactors, 0)
				continue
			}

			pf, err := performanceFactorStorage.Load(accountID)
			if err != nil {
				panic(ierrors.Wrapf(err, "failed to load performance factor for account %s", accountID))
			}

			intermediateFactors = append(intermediateFactors, pf)
		}

		if err := rewardsTree.Set(accountID, &PoolRewards{
			PoolStake:   pool.PoolStake,
			PoolRewards: t.poolReward(epochEndSlot, committee.TotalValidatorStake(), committee.TotalStake(), pool.PoolStake, pool.ValidatorStake, pool.FixedCost, t.aggregatePerformanceFactors(intermediateFactors)),
			FixedCost:   pool.FixedCost,
		}); err != nil {
			panic(ierrors.Wrapf(err, "failed to set rewards for account %s", accountID))
		}

		return true
	})

	if err := rewardsTree.Commit(); err != nil {
		panic(ierrors.Wrapf(err, "failed to commit rewards for epoch %d", epoch))
	}

	t.latestAppliedEpoch = epoch
}

func (t *Tracker) EligibleValidatorCandidates(_ iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for, only active

	return ds.NewSet[iotago.AccountID]()
}

// ValidatorCandidates returns the registered validator candidates for the given epoch.
func (t *Tracker) ValidatorCandidates(_ iotago.EpochIndex) ds.Set[iotago.AccountID] {
	// TODO: we should choose candidates we tracked performance for no matter if they were active

	return ds.NewSet[iotago.AccountID]()
}

func (t *Tracker) LoadCommitteeForEpoch(epoch iotago.EpochIndex) (committee *account.Accounts, exists bool) {
	c, err := t.committeeStore.Get(epoch)
	if err != nil {
		if ierrors.Is(err, kvstore.ErrKeyNotFound) {
			return nil, false
		}
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	return c, true
}

func (t *Tracker) aggregatePerformanceFactors(issuedBlocksPerSlot []uint64) uint64 {
	if len(issuedBlocksPerSlot) == 0 {
		return 0
	}

	var sum uint64
	for _, issuedBlocks := range issuedBlocksPerSlot {
		if issuedBlocks > uint64(validatorBlocksPerSlot) {
			// we harshly punish validators that issue any blocks more than allowed
			return 0
		}
		sum += issuedBlocks
	}

	// TODO: we should scale the result by the amount of slots per epoch,
	// otherwise we lose a lot of precision here.
	return sum / uint64(len(issuedBlocksPerSlot))
}
