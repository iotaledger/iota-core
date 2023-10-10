package poa

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SeatManager is a sybil protection module for the engine that manages the weights of actors according to their stake.
type SeatManager struct {
	events      *seatmanager.Events
	apiProvider iotago.APIProvider

	accounts         *account.Accounts
	committee        *account.SeatedAccounts
	committeeStore   *epochstore.Store[*account.Accounts]
	onlineCommittee  ds.Set[account.SeatIndex]
	inactivityQueue  timed.PriorityQueue[account.SeatIndex]
	lastActivities   *shrinkingmap.ShrinkingMap[account.SeatIndex, time.Time]
	lastActivityTime time.Time
	activityMutex    syncutils.RWMutex
	committeeMutex   syncutils.RWMutex

	optsActivityWindow         time.Duration
	optsOnlineCommitteeStartup []iotago.AccountID

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfAuthority module.
func NewProvider(opts ...options.Option[SeatManager]) module.Provider[*engine.Engine, seatmanager.SeatManager] {
	return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
		return options.Apply(
			&SeatManager{
				events:          seatmanager.NewEvents(),
				apiProvider:     e,
				committeeStore:  e.Storage.Committee(),
				accounts:        account.NewAccounts(),
				onlineCommittee: ds.NewSet[account.SeatIndex](),
				inactivityQueue: timed.NewPriorityQueue[account.SeatIndex](true),
				lastActivities:  shrinkingmap.New[account.SeatIndex, time.Time](),

				optsActivityWindow: time.Second * 30,
			}, opts, func(s *SeatManager) {
				e.Events.SeatManager.LinkTo(s.events)

				e.HookConstructed(func() {
					s.TriggerConstructed()

					// We need to mark validators as active upon solidity of blocks as otherwise we would not be able to
					// recover if no node was part of the online committee anymore.
					e.Events.CommitmentFilter.BlockAllowed.Hook(func(block *blocks.Block) {
						// Only track identities that are part of the committee.
						committee, exists := s.CommitteeInSlot(block.ID().Slot())
						if !exists {
							panic(ierrors.Errorf("committee not selected for slot %d, but received block in that slot", block.ID().Slot()))
						}

						seat, exists := committee.GetSeat(block.ProtocolBlock().IssuerID)
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

func (s *SeatManager) RotateCommittee(epoch iotago.EpochIndex, _ *account.Accounts) (*account.SeatedAccounts, error) {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	err := s.committeeStore.Store(epoch, s.committee.Accounts())
	if err != nil {
		return nil, ierrors.Wrapf(err, "error while storing committee for epoch %d", epoch)
	}

	return s.committee, nil
}

// CommitteeInSlot returns the set of validators selected to be part of the committee in the given slot.
func (s *SeatManager) CommitteeInSlot(slot iotago.SlotIndex) (*account.SeatedAccounts, bool) {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	return s.committeeInEpoch(s.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot))
}

// CommitteeInEpoch returns the set of validators selected to be part of the committee in the given epoch.
func (s *SeatManager) CommitteeInEpoch(epoch iotago.EpochIndex) (*account.SeatedAccounts, bool) {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	return s.committeeInEpoch(epoch)
}

func (s *SeatManager) committeeInEpoch(epoch iotago.EpochIndex) (*account.SeatedAccounts, bool) {
	c, err := s.committeeStore.Load(epoch)
	if err != nil {
		panic(ierrors.Wrapf(err, "failed to load committee for epoch %d", epoch))
	}

	if c == nil {
		return nil, false
	}

	return c.SelectCommittee(c.IDs()...), true
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
}

func (s *SeatManager) InitializeCommittee(epoch iotago.EpochIndex, activityTime time.Time) error {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	committeeAccounts, err := s.committeeStore.Load(epoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to load PoA committee for epoch %d", epoch)
	}

	s.accounts = committeeAccounts
	s.committee = s.accounts.SelectCommittee(committeeAccounts.IDs()...)

	onlineValidators := s.accounts.IDs()
	if len(s.optsOnlineCommitteeStartup) > 0 {
		onlineValidators = s.optsOnlineCommitteeStartup
	}

	for _, v := range onlineValidators {
		seat, exists := s.committee.GetSeat(v)
		if !exists {
			// Only track identities that are part of the committee.
			continue
		}

		s.markSeatActive(seat, v, activityTime)
	}

	return nil
}

func (s *SeatManager) SetCommittee(epoch iotago.EpochIndex, validators *account.Accounts) error {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	s.accounts = validators
	s.committee = s.accounts.SelectCommittee(validators.IDs()...)

	err := s.committeeStore.Store(epoch, s.accounts)
	if err != nil {
		return ierrors.Wrapf(err, "failed to set committee for epoch %d", epoch)
	}

	return nil
}

func (s *SeatManager) markSeatActive(seat account.SeatIndex, id iotago.AccountID, seatActivityTime time.Time) {
	s.activityMutex.Lock()
	defer s.activityMutex.Unlock()

	if lastActivity, exists := s.lastActivities.Get(seat); (exists && lastActivity.After(seatActivityTime)) || seatActivityTime.Before(s.lastActivityTime.Add(-s.optsActivityWindow)) {
		return
	} else if !exists {
		s.onlineCommittee.Add(seat)
		s.events.OnlineCommitteeSeatAdded.Trigger(seat, id)
	}

	s.lastActivities.Set(seat, seatActivityTime)

	s.inactivityQueue.Push(seat, seatActivityTime)

	if seatActivityTime.Before(s.lastActivityTime) {
		return
	}

	s.lastActivityTime = seatActivityTime

	activityThreshold := seatActivityTime.Add(-s.optsActivityWindow)
	for _, inactiveSeat := range s.inactivityQueue.PopUntil(activityThreshold) {
		if lastActivityForInactiveSeat, exists := s.lastActivities.Get(inactiveSeat); exists && lastActivityForInactiveSeat.After(activityThreshold) {
			continue
		}

		s.markSeatInactive(inactiveSeat)
	}
}

func (s *SeatManager) markSeatInactive(seat account.SeatIndex) {
	s.lastActivities.Delete(seat)
	s.onlineCommittee.Delete(seat)

	s.events.OnlineCommitteeSeatRemoved.Trigger(seat)
}
