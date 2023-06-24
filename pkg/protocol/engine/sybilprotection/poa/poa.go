package poa

import (
	"sync"
	"time"

	"github.com/iotaledger/hive.go/ds/advancedset"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/sybilprotection"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SybilProtection is a sybil protection module for the engine that manages the weights of actors according to their stake.
type SybilProtection struct {
	events *sybilprotection.Events

	clock             clock.Clock
	timeProviderFunc  func() *iotago.TimeProvider
	workers           *workerpool.Group
	accounts          *account.Accounts
	committee         *account.SeatedAccounts
	onlineCommittee   *advancedset.AdvancedSet[account.SeatIndex]
	inactivityManager *timed.TaskExecutor[iotago.AccountID]
	lastActivities    *shrinkingmap.ShrinkingMap[account.SeatIndex, time.Time]
	mutex             sync.RWMutex

	optsActivityWindow         time.Duration
	optsOnlineCommitteeStartup []iotago.AccountID

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfStake module.
func NewProvider(validators []iotago.AccountID, opts ...options.Option[SybilProtection]) module.Provider[*engine.Engine, sybilprotection.SybilProtection] {
	return module.Provide(func(e *engine.Engine) sybilprotection.SybilProtection {
		return options.Apply(
			&SybilProtection{
				events:            sybilprotection.NewEvents(),
				workers:           e.Workers.CreateGroup("SybilProtection"),
				accounts:          account.NewAccounts(),
				inactivityManager: timed.NewTaskExecutor[iotago.AccountID](1),
				lastActivities:    shrinkingmap.New[account.SeatIndex, time.Time](),

				optsActivityWindow:         time.Second * 30,
				optsOnlineCommitteeStartup: validators,
			}, opts, func(s *SybilProtection) {
				e.Events.SybilProtection.LinkTo(s.events)

				s.initializeAccounts(validators)
				s.onlineCommittee = advancedset.New[account.SeatIndex]()

				e.HookConstructed(func() {
					s.clock = e.Clock

					e.Storage.Settings().HookInitialized(func() {
						s.timeProviderFunc = e.API().TimeProvider

						e.Clock.HookInitialized(func() {
							for _, v := range s.optsOnlineCommitteeStartup {
								s.markValidatorActive(v, e.Clock.Accepted().RelativeTime())
							}
						})
					})

					// We need to mark validators as active upon solidity of blocks as otherwise we would not be able to
					// recover if no node was part of the online committee anymore.
					e.Events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
						s.markValidatorActive(block.Block().IssuerID, block.IssuingTime())
						s.events.BlockProcessed.Trigger(block)
					})
				})
			})
	})
}

var _ sybilprotection.SybilProtection = &SybilProtection{}

func (s *SybilProtection) RotateCommittee(_ iotago.EpochIndex, _ *account.Accounts) *account.SeatedAccounts {
	// we do nothing on PoA, we keep the same accounts and committee
	return s.committee
}

// Committee returns the set of validators selected to be part of the committee.
func (s *SybilProtection) Committee(_ iotago.SlotIndex) *account.SeatedAccounts {
	// Note: we have PoA so our committee do not rotate right now
	return s.committee
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (s *SybilProtection) OnlineCommittee() *advancedset.AdvancedSet[account.SeatIndex] {
	return s.onlineCommittee
}

func (s *SybilProtection) SeatCount() int {
	return s.committee.SeatCount()
}

func (s *SybilProtection) Shutdown() {
	s.TriggerStopped()
	s.stopInactivityManager()
	s.workers.Shutdown()
}

func (s *SybilProtection) initializeAccounts(validators []iotago.AccountID) {
	for _, id := range validators {
		s.accounts.Set(id, &account.Pool{}) // We do not care about the pool with PoA
	}
	s.committee = s.accounts.SelectCommittee(validators...)
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

	slotIndex := s.timeProviderFunc().SlotFromTime(activityTime)
	seat, exists := s.Committee(slotIndex).GetSeat(id)
	if !exists {
		// Only track identities that are part of the committee
		return
	}

	if lastActivity, exists := s.lastActivities.Get(seat); exists && lastActivity.After(activityTime) {
		return
	} else if !exists {
		s.onlineCommittee.Add(seat)
		s.events.OnlineCommitteeSeatAdded.Trigger(seat, id)
	}

	s.lastActivities.Set(seat, activityTime)

	s.inactivityManager.ExecuteAfter(id, func() { s.markSeatInactive(seat) }, activityTime.Add(s.optsActivityWindow).Sub(s.clock.Accepted().RelativeTime()))
}

func (s *SybilProtection) markSeatInactive(seat account.SeatIndex) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.lastActivities.Delete(seat)
	s.onlineCommittee.Delete(seat)

	s.events.OnlineCommitteeSeatRemoved.Trigger(seat)
}
