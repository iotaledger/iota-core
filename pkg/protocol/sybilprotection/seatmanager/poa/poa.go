package poa

import (
	"fmt"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/hive.go/runtime/workerpool"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/clock"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SeatManager is a sybil protection module for the engine that manages the weights of actors according to their stake.
type SeatManager struct {
	events *seatmanager.Events

	clock             clock.Clock
	timeProviderFunc  func() *iotago.TimeProvider
	workers           *workerpool.Group
	accounts          *account.Accounts
	committee         *account.SeatedAccounts
	onlineCommittee   ds.Set[account.SeatIndex]
	inactivityManager *timed.TaskExecutor[account.SeatIndex]
	lastActivities    *shrinkingmap.ShrinkingMap[account.SeatIndex, time.Time]
	activityMutex     syncutils.RWMutex
	committeeMutex    syncutils.RWMutex

	optsActivityWindow         time.Duration
	optsOnlineCommitteeStartup []iotago.AccountID

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfStake module.
func NewProvider(opts ...options.Option[SeatManager]) module.Provider[*engine.Engine, seatmanager.SeatManager] {
	return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
		return options.Apply(
			&SeatManager{
				events:            seatmanager.NewEvents(),
				workers:           e.Workers.CreateGroup("SeatManager"),
				accounts:          account.NewAccounts(),
				onlineCommittee:   ds.NewSet[account.SeatIndex](),
				inactivityManager: timed.NewTaskExecutor[account.SeatIndex](1),
				lastActivities:    shrinkingmap.New[account.SeatIndex, time.Time](),

				optsActivityWindow: time.Second * 30,
			}, opts, func(s *SeatManager) {
				e.Events.SeatManager.LinkTo(s.events)

				e.HookConstructed(func() {
					s.clock = e.Clock

					s.timeProviderFunc = func() *iotago.TimeProvider {
						return e.CurrentAPI().TimeProvider()
					}
					s.TriggerConstructed()

					// We need to mark validators as active upon solidity of blocks as otherwise we would not be able to
					// recover if no node was part of the online committee anymore.
					e.Events.CommitmentFilter.BlockAllowed.Hook(func(block *blocks.Block) {
						// Only track identities that are part of the committee.
						seat, exists := s.Committee(block.ID().Index()).GetSeat(block.ProtocolBlock().IssuerID)
						if exists {
							s.markSeatActive(seat, block.ProtocolBlock().IssuerID, block.IssuingTime())
						}

						s.events.BlockProcessed.Trigger(block)
					})
				})
			})
	})
}

var _ seatmanager.SeatManager = &SeatManager{}

func (s *SeatManager) RotateCommittee(_ iotago.EpochIndex, _ *account.Accounts) *account.SeatedAccounts {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	// we do nothing on PoA, we keep the same accounts and committee
	return s.committee
}

// Committee returns the set of validators selected to be part of the committee.
func (s *SeatManager) Committee(_ iotago.SlotIndex) *account.SeatedAccounts {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	// Note: we have PoA so our committee do not rotate right now
	return s.committee
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (s *SeatManager) OnlineCommittee() ds.Set[account.SeatIndex] {
	s.activityMutex.RLock()
	defer s.activityMutex.RUnlock()

	return s.onlineCommittee
}

func (s *SeatManager) SeatCount() int {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	return s.committee.SeatCount()
}

func (s *SeatManager) Shutdown() {
	s.TriggerStopped()
	s.stopInactivityManager()
	s.workers.Shutdown()
}

func (s *SeatManager) ImportCommittee(_ iotago.EpochIndex, validators *account.Accounts) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	s.accounts = validators
	s.committee = s.accounts.SelectCommittee(validators.IDs()...)

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

func (s *SeatManager) SetCommittee(_ iotago.EpochIndex, validators *account.Accounts) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	s.accounts = validators
	s.committee = s.accounts.SelectCommittee(validators.IDs()...)
}

func (s *SeatManager) stopInactivityManager() {
	s.inactivityManager.Shutdown(timed.CancelPendingElements)
}

func (s *SeatManager) markSeatActive(seat account.SeatIndex, id iotago.AccountID, activityTime time.Time) {
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
	fmt.Println("markSeatActive @ ", time.Now(), "seat:", seat, id, "activity time:", activityTime, "relative accepted time:", s.clock.Accepted().RelativeTime(), "mark inactive at", activityTime.Add(s.optsActivityWindow).Sub(s.clock.Accepted().RelativeTime()))
}

func (s *SeatManager) markSeatInactive(seat account.SeatIndex) {
	s.activityMutex.Lock()
	defer s.activityMutex.Unlock()

	s.lastActivities.Delete(seat)
	s.onlineCommittee.Delete(seat)

	s.events.OnlineCommitteeSeatRemoved.Trigger(seat)
}
