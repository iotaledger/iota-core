package topstakers

import (
	"bytes"
	"sort"
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/hive.go/runtime/module"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/engine"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/accounts"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/blocks"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/activitytracker"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/activitytracker/activitytrackerv1"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/seatmanager"
	"github.com/iotaledger/iota-core/pkg/storage/prunable/epochstore"
	iotago "github.com/iotaledger/iota.go/v4"
)

// SeatManager is a sybil protection module for the engine that manages the weights of actors according to their stake.
type SeatManager struct {
	apiProvider iotago.APIProvider
	events      *seatmanager.Events

	committeeStore  epochstore.Store[*account.SeatedAccounts]
	committeeMutex  syncutils.RWMutex
	activityTracker activitytracker.ActivityTracker

	optsOnlineCommitteeStartup []iotago.AccountID

	module.Module
}

// NewProvider returns a new sybil protection provider that uses the ProofOfStake module.
func NewProvider(opts ...options.Option[SeatManager]) module.Provider[*engine.Engine, seatmanager.SeatManager] {
	return module.Provide(func(e *engine.Engine) seatmanager.SeatManager {
		return options.Apply(
			&SeatManager{
				apiProvider:    e,
				events:         seatmanager.NewEvents(),
				committeeStore: e.Storage.Committee(),
			}, opts, func(s *SeatManager) {
				activityTracker := activitytrackerv1.NewActivityTracker(e)
				s.activityTracker = activityTracker
				s.events.OnlineCommitteeSeatAdded.LinkTo(activityTracker.Events.OnlineCommitteeSeatAdded)
				s.events.OnlineCommitteeSeatRemoved.LinkTo(activityTracker.Events.OnlineCommitteeSeatRemoved)

				e.Events.SeatManager.LinkTo(s.events)

				e.Constructed.OnTrigger(func() {
					s.TriggerConstructed()

					// We need to mark validators as active upon solidity of blocks as otherwise we would not be able to
					// recover if no node was part of the online committee anymore.
					e.Events.PostSolidFilter.BlockAllowed.Hook(func(block *blocks.Block) {
						// Only track identities that are part of the committee.
						committee, exists := s.CommitteeInSlot(block.ID().Slot())
						if !exists {
							panic(ierrors.Errorf("committee not selected for slot %d, but received block in that slot", block.ID().Slot()))
						}

						seat, exists := committee.GetSeat(block.ProtocolBlock().Header.IssuerID)
						if exists {
							s.activityTracker.MarkSeatActive(seat, block.ProtocolBlock().Header.IssuerID, block.IssuingTime())
						}

						s.events.BlockProcessed.Trigger(block)
					})
				})
			})
	})
}

var _ seatmanager.SeatManager = &SeatManager{}

func (s *SeatManager) RotateCommittee(epoch iotago.EpochIndex, candidates accounts.AccountsData) (*account.SeatedAccounts, error) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	if len(candidates) == 0 {
		return nil, ierrors.New("candidates must not be empty")
	}

	committeeAccounts, err := s.selectNewCommitteeAccounts(epoch, candidates)
	if err != nil {
		return nil, ierrors.Wrap(err, "error while selecting new committee")
	}

	// Load previous committee only if target epoch is not zero.
	var prevCommittee *account.SeatedAccounts
	if epoch > 0 {
		prevCommittee, err = s.committeeStore.Load(epoch - 1)
		if err != nil {
			return nil, err
		}
	}

	// If there is no previous committee , then we can assign seats by default.
	if prevCommittee == nil {
		newCommittee := committeeAccounts.SeatedAccounts()

		if err := s.committeeStore.Store(epoch, newCommittee); err != nil {
			return nil, ierrors.Wrapf(err, "error while storing committee for epoch %d", epoch)
		}

		return newCommittee, nil
	}

	newCommittee := committeeAccounts.SeatedAccounts(prevCommittee)

	if err := s.committeeStore.Store(epoch, newCommittee); err != nil {
		return nil, ierrors.Wrapf(err, "error while storing committee for epoch %d", epoch)
	}

	return newCommittee, nil
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

	return c, true
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (s *SeatManager) OnlineCommittee() ds.Set[account.SeatIndex] {
	return s.activityTracker.OnlineCommittee()
}

func (s *SeatManager) SeatCountInSlot(slot iotago.SlotIndex) int {
	epoch := s.apiProvider.APIForSlot(slot).TimeProvider().EpochFromSlot(slot)

	return s.SeatCountInEpoch(epoch)
}

func (s *SeatManager) SeatCountInEpoch(epoch iotago.EpochIndex) int {
	s.committeeMutex.RLock()
	defer s.committeeMutex.RUnlock()

	// TODO: this function is a hot path as it is called for every single block. Maybe accessing the storage is too slow.
	if committee, exists := s.committeeInEpoch(epoch); exists {
		return committee.SeatCount()
	}

	return int(s.apiProvider.APIForEpoch(epoch).ProtocolParameters().TargetCommitteeSize())
}

func (s *SeatManager) Shutdown() {
	s.TriggerStopped()
}

func (s *SeatManager) InitializeCommittee(epoch iotago.EpochIndex, activityTime time.Time) error {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	committee, err := s.committeeStore.Load(epoch)
	if err != nil {
		return ierrors.Wrapf(err, "failed to load PoA committee for epoch %d", epoch)
	}

	committeeAccounts, err := committee.Accounts()
	if err != nil {
		return ierrors.Wrapf(err, "failed to retrieve accounts of the committee for epoch %d", epoch)
	}

	onlineValidators := committeeAccounts.IDs()

	if len(s.optsOnlineCommitteeStartup) > 0 {
		onlineValidators = s.optsOnlineCommitteeStartup
	}

	for _, v := range onlineValidators {
		seat, exists := committee.GetSeat(v)
		if !exists {
			// Only track identities that are part of the committee.
			continue
		}

		s.activityTracker.MarkSeatActive(seat, v, activityTime)
	}

	return nil
}

func (s *SeatManager) ReuseCommittee(currentEpoch iotago.EpochIndex, targetEpoch iotago.EpochIndex) (*account.SeatedAccounts, error) {
	s.committeeMutex.Lock()
	defer s.committeeMutex.Unlock()

	currentCommittee, exists := s.committeeInEpoch(currentEpoch)
	if !exists {
		// that should never happen as it is already the fallback strategy
		panic(ierrors.Errorf("committee for current epoch %d not found", currentEpoch))
	}

	if currentCommittee.SeatCount() == 0 {
		return nil, ierrors.New("committee must not be empty")
	}

	newCommittee, err := currentCommittee.Reuse()
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to reuse committee from epoch %d", currentEpoch)
	}

	err = s.committeeStore.Store(targetEpoch, newCommittee)
	if err != nil {
		return nil, ierrors.Wrapf(err, "failed to set committee for epoch %d", targetEpoch)
	}

	return newCommittee, nil
}

func (s *SeatManager) selectNewCommitteeAccounts(epoch iotago.EpochIndex, candidates accounts.AccountsData) (*account.Accounts, error) {
	sort.Slice(candidates, func(i int, j int) bool {
		// Prioritize the candidate that has a larger pool stake.
		if candidates[i].ValidatorStake+candidates[i].DelegationStake != candidates[j].ValidatorStake+candidates[j].DelegationStake {
			return candidates[i].ValidatorStake+candidates[i].DelegationStake > candidates[j].ValidatorStake+candidates[j].DelegationStake
		}

		// Prioritize the candidate that has a larger validator stake.
		if candidates[i].ValidatorStake != candidates[j].ValidatorStake {
			return candidates[i].ValidatorStake > candidates[j].ValidatorStake
		}

		// Prioritize the candidate that declares a longer staking period.
		if candidates[i].StakeEndEpoch != candidates[j].StakeEndEpoch {
			return candidates[i].StakeEndEpoch > candidates[j].StakeEndEpoch
		}

		// Prioritize the candidate that has smaller FixedCost.
		if candidates[i].FixedCost != candidates[j].FixedCost {
			return candidates[i].FixedCost < candidates[j].FixedCost
		}

		// two candidates never have the same account ID because they come in a map
		return bytes.Compare(candidates[i].ID[:], candidates[j].ID[:]) > 0
	})

	// We try to select up to targetCommitteeSize candidates to be part of the committee. If there are fewer candidates
	// than required, then we select all of them and the committee size will be smaller than targetCommitteeSize.
	committeeSize := lo.Min(len(candidates), int(s.apiProvider.APIForEpoch(epoch).ProtocolParameters().TargetCommitteeSize()))

	// Create new Accounts instance that only included validators selected to be part of the committee.
	newCommitteeAccounts := account.NewAccounts()

	for _, candidateData := range candidates[:committeeSize] {
		if err := newCommitteeAccounts.Set(candidateData.ID, &account.Pool{
			PoolStake:      candidateData.ValidatorStake + candidateData.DelegationStake,
			ValidatorStake: candidateData.ValidatorStake,
			FixedCost:      candidateData.FixedCost,
		}); err != nil {
			return nil, ierrors.Wrapf(err, "error while setting pool for committee candidate %s", candidateData.ID.String())
		}
	}

	return newCommitteeAccounts, nil
}
