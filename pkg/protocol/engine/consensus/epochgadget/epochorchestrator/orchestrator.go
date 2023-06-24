package epochorchestrator

import (
	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/ledger"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Orchestrator struct {
	sybilProtection    sybilprotection.SybilProtection
	performanceManager PerformanceManager
	timeProvider       func() *iotago.TimeProvider
	ledger             ledger.Ledger

	optsEpochEndNearingThreshold iotago.SlotIndex
}

type PerformanceManager interface {
	EligibleValidatorCandidates(epochIndex iotago.EpochIndex) *advancedset.AdvancedSet[iotago.AccountID]
	RegisterCommittee(epochIndex iotago.EpochIndex, committee *account.Accounts)
	ApplyEpoch(epochIndex iotago.EpochIndex)
}

func (o *Orchestrator) CheckEpochEndNearing(index iotago.SlotIndex) {
	currentEpoch := o.timeProvider().EpochsFromSlot(index)
	nextEpoch := currentEpoch + 1
	if o.timeProvider().EpochEnd(currentEpoch)-o.optsEpochEndNearingThreshold == index { //epoch
		candidates := o.performanceManager.EligibleValidatorCandidates(nextEpoch) // epoch

		weightedCandidates := account.NewAccounts()
		if err := candidates.ForEach(func(candidate iotago.AccountID) error {
			a, exists, err := o.ledger.Account(candidate)
			if err != nil {
				return err
			}
			if !exists {
				panic("account does not exist")
			}

			weightedCandidates.Set(candidate, &account.Pool{
				PoolStake:      a.ValidatorStake + a.DelegationStake,
				ValidatorStake: a.ValidatorStake,
				FixedCost:      a.FixedCost,
			})

			return nil
		}); err != nil {
			panic(err)
		}

		newCommittee := o.sybilProtection.RotateCommittee(nextEpoch, weightedCandidates)
		weightedCommittee := newCommittee.Accounts()
		o.performanceManager.RegisterCommittee(nextEpoch, weightedCommittee)
	}
}

func (o *Orchestrator) CheckEpochEnd(index iotago.SlotIndex) {
	currentEpoch := o.timeProvider().EpochsFromSlot(index)
	if o.timeProvider().EpochEnd(currentEpoch) == index {
		o.performanceManager.ApplyEpoch(currentEpoch)
	}
}
