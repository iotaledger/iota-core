package poa

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/core/account"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

const (
	PrefixLastCommittedSlot byte = iota
	PrefixWeights
)

// SybilProtection is a sybil protection module for the engine that manages the weights of actors according to their stake.
type SybilProtection struct {
	clock             clock.Clock
	workers           *workerpool.Group
	accounts          *account.Accounts[iotago.AccountID, *iotago.AccountID]
	onlineComittee    *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID]
	inactivityManager *timed.TaskExecutor[iotago.AccountID]
	lastActivities    *shrinkingmap.ShrinkingMap[iotago.AccountID, time.Time]
	mutex             sync.RWMutex

	optsActivityWindow time.Duration

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfStake module.
func NewProvider(weightVector map[iotago.AccountID]int64, opts ...options.Option[SybilProtection]) module.Provider[*engine.Engine, sybilprotection.SybilProtection] {
	return module.Provide(func(e *engine.Engine) sybilprotection.SybilProtection {
		return options.Apply(
			&SybilProtection{
				workers:           e.Workers.CreateGroup("SybilProtection"),
				accounts:          account.NewAccounts[iotago.AccountID](e.Storage.SybilProtection(PrefixWeights)),
				inactivityManager: timed.NewTaskExecutor[iotago.AccountID](1),
				lastActivities:    shrinkingmap.New[iotago.AccountID, time.Time](),

				optsActivityWindow: time.Second * 30,
			}, opts, func(s *SybilProtection) {
				s.initializeAccounts(weightVector)
				s.onlineComittee = s.accounts.SelectAccounts()

				e.HookConstructed(func() {
					e.HookStopped(s.stopInactivityManager)

					s.clock = e.Clock

					e.Clock.HookInitialized(func() {
						for v := range weightVector {
							s.markValidatorActive(v, e.Clock.Accepted().RelativeTime())
						}
					})

					e.Events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
						s.markValidatorActive(block.Block().IssuerID, block.IssuingTime())
					}, event.WithWorkerPool(s.workers.CreatePool("SybilProtection", 1)))
				})
			})
	})
}

var _ sybilprotection.SybilProtection = &SybilProtection{}

// Accounts returns all the known validators.
func (s *SybilProtection) Accounts() *account.Accounts[iotago.AccountID, *iotago.AccountID] {
	return s.accounts
}

// Committee returns the set of validators selected to be part of the committee.
func (s *SybilProtection) Committee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID] {
	return s.accounts.SelectAccounts(lo.Keys(lo.PanicOnErr(s.accounts.Map()))...)
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (s *SybilProtection) OnlineCommittee() *account.SelectedAccounts[iotago.AccountID, *iotago.AccountID] {
	return s.onlineComittee
}

func (s *SybilProtection) LastCommittedSlot() iotago.SlotIndex {
	return 0
}

func (s *SybilProtection) Shutdown() {
	s.stopInactivityManager()
	s.TriggerStopped()
}

func (s *SybilProtection) initializeAccounts(weightVector map[iotago.AccountID]int64) {
	for id, weight := range weightVector {
		s.accounts.Update(id, weight)
	}
}

func (s *SybilProtection) stopInactivityManager() {
	s.inactivityManager.Shutdown(timed.CancelPendingElements)
}

func (s *SybilProtection) markValidatorActive(id iotago.AccountID, activityTime time.Time) {
	if s.clock.WasStopped() {
		return
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.Committee().Has(id) {
		// Only track identities that are part of the committee
		return
	}

	if lastActivity, exists := s.lastActivities.Get(id); exists && lastActivity.After(activityTime) {
		return
	} else if !exists {
		s.onlineComittee.Add(id)
	}

	s.lastActivities.Set(id, activityTime)

	s.inactivityManager.ExecuteAfter(id, func() { s.markValidatorInactive(id) }, activityTime.Add(s.optsActivityWindow).Sub(s.clock.Accepted().RelativeTime()))
}

func (s *SybilProtection) markValidatorInactive(id iotago.AccountID) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.lastActivities.Delete(id)
	s.onlineComittee.Delete(id)
}
