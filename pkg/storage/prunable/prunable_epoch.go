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
	pruningDelayPoolRewards           = 365
	pruningDelayPoolStats             = 365
	pruningDelayCommittee             = 365
)

func (p *Prunable) Rewards(epoch iotago.EpochIndex) kvstore.KVStore {
	return p.poolRewards.GetEpoch(epoch)
}

func (p *Prunable) PoolStats() *epochstore.Store[*model.PoolsStats] {
	return p.poolStats
}

func (p *Prunable) DecidedUpgradeSignals() *epochstore.Store[model.VersionAndHash] {
	return p.decidedUpgradeSignals
}

func (p *Prunable) Committee() *epochstore.Store[*account.Accounts] {
	return p.committee
}
