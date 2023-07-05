package poa

import (
	"fmt"
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
	inactivityManager *timed.TaskExecutor[account.SeatIndex]
	lastActivities    *shrinkingmap.ShrinkingMap[account.SeatIndex, time.Time]
	activityMutex     sync.RWMutex
	committeeMutex    sync.RWMutex

	optsActivityWindow         time.Duration
	optsOnlineCommitteeStartup []iotago.AccountID

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfStake module.
func NewProvider(opts ...options.Option[SybilProtection]) module.Provider[*engine.Engine, sybilprotection.SybilProtection] {
	return module.Provide(func(e *engine.Engine) sybilprotection.SybilProtection {
		return options.Apply(
			&SybilProtection{
				events:            sybilprotection.NewEvents(),
				workers:           e.Workers.CreateGroup("SybilProtection"),
				accounts:          account.NewAccounts(),
				inactivityManager: timed.NewTaskExecutor[account.SeatIndex](1),
				lastActivities:    shrinkingmap.New[account.SeatIndex, time.Time](),

				optsActivityWindow: time.Second * 30,
			}, opts, func(s *SybilProtection) {
				e.Events.SybilProtection.LinkTo(s.events)

				s.onlineCommittee = advancedset.New[account.SeatIndex]()

				e.HookConstructed(func() {
					s.clock = e.Clock

					e.Storage.Settings().HookInitialized(func() {
						s.timeProviderFunc = e.API().TimeProvider
						s.TriggerConstructed()

						// We need to mark validators as active upon solidity of blocks as otherwise we would not be able to
						// recover if no node was part of the online committee anymore.
						e.Events.BlockDAG.BlockSolid.Hook(func(block *blocks.Block) {
							seat, exists := s.Committee(block.ID().Index()).GetSeat(block.Block().IssuerID)
							if !exists {
								// Only track identities that are part of the committee.
								return
							}

							s.markSeatActive(seat, block.Block().IssuerID, block.IssuingTime())

							s.events.BlockProcessed.Trigger(block)
						})
					})
				})
			})
	})
}

var _ sybilprotection.SybilProtection = &SybilProtection{}

func (s *SybilProtection) RotateCommittee(_ iotago.EpochIndex, _ *account.Accounts) *account.SeatedAccounts {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	// we do nothing on PoA, we keep the same accounts and committee
	return s.committee
}

// Committee returns the set of validators selected to be part of the committee.
func (s *SybilProtection) Committee(_ iotago.SlotIndex) *account.SeatedAccounts {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	// Note: we have PoA so our committee do not rotate right now
	return s.committee
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (s *SybilProtection) OnlineCommittee() *advancedset.AdvancedSet[account.SeatIndex] {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()

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

func (s *SybilProtection) ImportCommittee(_ iotago.EpochIndex, validators *account.Accounts) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	s.accounts = validators
	s.committee = s.accounts.SelectCommittee(validators.IDs()...)
	fmt.Println("import committee", validators.IDs())
	onlineValidators := s.accounts.IDs()
	if len(s.optsOnlineCommitteeStartup) > 0 {
		onlineValidators = s.optsOnlineCommitteeStartup
	}

	for _, v := range onlineValidators {
		activityTime := s.clock.Accepted().RelativeTime()

		seat, exists := s.committee.GetSeat(v)
		if !exists {
			// Only track identities that are part of the committee.
			return
		}

		s.markSeatActive(seat, v, activityTime)
	}
}

func (s *SybilProtection) SetCommittee(_ iotago.EpochIndex, validators *account.Accounts) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	s.accounts = validators
	s.committee = s.accounts.SelectCommittee(validators.IDs()...)

	fmt.Println("set committee", validators.IDs())
}

func (s *SybilProtection) stopInactivityManager() {
	s.inactivityManager.Shutdown(timed.CancelPendingElements)
}

func (s *SybilProtection) markSeatActive(seat account.SeatIndex, id iotago.AccountID, activityTime time.Time) {
	if s.clock.WasStopped() {
		return
	}

	s.activityMutex.Lock()
	defer s.activityMutex.Unlock()

	if lastActivity, exists := s.lastActivities.Get(seat); exists && lastActivity.After(activityTime) {
		return
	} else if !exists {
		s.onlineCommittee.Add(seat)
		s.events.OnlineCommitteeSeatAdded.Trigger(seat, id)
	}

	s.lastActivities.Set(seat, activityTime)

	s.inactivityManager.ExecuteAfter(seat, func() { s.markSeatInactive(seat) }, activityTime.Add(s.optsActivityWindow).Sub(s.clock.Accepted().RelativeTime()))
}

func (s *SybilProtection) markSeatInactive(seat account.SeatIndex) {
	s.activityMutex.Lock()
	defer s.activityMutex.Unlock()

	s.lastActivities.Delete(seat)
	s.onlineCommittee.Delete(seat)
	fmt.Println("mark seat inactive", seat)

	s.events.OnlineCommitteeSeatRemoved.Trigger(seat)
}
