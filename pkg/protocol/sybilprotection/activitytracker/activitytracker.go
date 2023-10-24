package activitytracker

import (
	"time"

	"github.com/iotaledger/hive.go/ds"
	"github.com/iotaledger/iota-core/pkg/core/account"
	iotago "github.com/iotaledger/iota.go/v4"
)

type ActivityTracker interface {
	OnlineCommittee() ds.Set[account.SeatIndex]
	MarkSeatActive(seat account.SeatIndex, id iotago.AccountID, seatActivityTime time.Time)
}
