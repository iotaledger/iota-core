package activitytrackerv1

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/hive.go/ds/shrinkingmap"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/hive.go/runtime/timed"
	"github.com/iotaledger/iota-core/pkg/core/account"
	"github.com/iotaledger/iota-core/pkg/protocol/sybilprotection/activitytracker"
	iotago "github.com/iotaledger/iota.go/v4"
)

// ActivityTracker is a sybil protection module for tracking activity of committee members.
type ActivityTracker struct {
	Events *activitytracker.Events

	onlineCommittee  ds.Set[account.SeatIndex]
	inactivityQueue  timed.PriorityQueue[account.SeatIndex]
	lastActivities   *shrinkingmap.ShrinkingMap[account.SeatIndex, time.Time]
	lastActivityTime time.Time
	activityMutex    syncutils.RWMutex

	apiProvider iotago.APIProvider
}

func NewActivityTracker(apiProvider iotago.APIProvider) *ActivityTracker {
	return &ActivityTracker{
		Events:          activitytracker.NewEvents(),
		onlineCommittee: ds.NewSet[account.SeatIndex](),
		inactivityQueue: timed.NewPriorityQueue[account.SeatIndex](true),
		lastActivities:  shrinkingmap.New[account.SeatIndex, time.Time](),

		apiProvider: apiProvider,
	}
}

// OnlineCommittee returns the set of validators selected to be part of the committee that has been seen recently.
func (a *ActivityTracker) OnlineCommittee() ds.Set[account.SeatIndex] {
	a.activityMutex.RLock()
	defer a.activityMutex.RUnlock()

	return a.onlineCommittee
}

func (a *ActivityTracker) MarkSeatActive(seat account.SeatIndex, id iotago.AccountID, seatActivityTime time.Time) {
	a.activityMutex.Lock()
	defer a.activityMutex.Unlock()

	// activity window is given by min committable age in seconds from the protocol parameters
	protocolParams := a.apiProvider.APIForTime(seatActivityTime).ProtocolParameters()
	activityWindow := time.Duration(protocolParams.MinCommittableAge()*iotago.SlotIndex(protocolParams.SlotDurationInSeconds())) * time.Second
	if lastActivity, exists := a.lastActivities.Get(seat); (exists && lastActivity.After(seatActivityTime)) || seatActivityTime.Before(a.lastActivityTime.Add(-activityWindow)) {
		return
	} else if !exists {
		a.onlineCommittee.Add(seat)
		a.Events.OnlineCommitteeSeatAdded.Trigger(seat, id)
	}

	a.lastActivities.Set(seat, seatActivityTime)

	a.inactivityQueue.Push(seat, seatActivityTime)

	if seatActivityTime.Before(a.lastActivityTime) {
		return
	}

	a.lastActivityTime = seatActivityTime

	activityThreshold := seatActivityTime.Add(-activityWindow)
	for _, inactiveSeat := range a.inactivityQueue.PopUntil(activityThreshold) {
		if lastActivityForInactiveSeat, exists := a.lastActivities.Get(inactiveSeat); exists && lastActivityForInactiveSeat.After(activityThreshold) {
			continue
		}

		a.markSeatInactive(inactiveSeat)
	}
}

func (a *ActivityTracker) markSeatInactive(seat account.SeatIndex) {
	a.lastActivities.Delete(seat)

	// Only trigger the event if online committee member is removed.
	if a.onlineCommittee.Delete(seat) {
		a.Events.OnlineCommitteeSeatRemoved.Trigger(seat)
	}
}
