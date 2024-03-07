package prunable

import (
	"github.com/iotaledger/hive.go/kvstore"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/model"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	epochPrefixDecidedUpgradeSignals byte = iota
	epochPrefixPoolRewards
	epochPrefixPoolStats
	epochPrefixCommittee
)

const (
	pruningDelayDecidedUpgradeSignals = 7
)

func (p *Prunable) RewardsForEpoch(epoch iotago.EpochIndex) (kvstore.KVStore, error) {
	return p.poolRewards.GetEpoch(epoch)
}

func (p *Prunable) Rewards() *epochstore.EpochKVStore {
	return p.poolRewards
}

func (p *Prunable) PoolStats() *epochstore.BaseStore[*model.PoolsStats] {
	return p.poolStats
}

func (p *Prunable) DecidedUpgradeSignals() *epochstore.BaseStore[model.VersionAndHash] {
	return p.decidedUpgradeSignals
}

func (p *Prunable) Committee() *epochstore.CachedStore[*account.SeatedAccounts] {
	return p.committee
}
