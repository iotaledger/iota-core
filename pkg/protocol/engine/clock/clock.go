package clock

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/module"
)

// Clock is an engine module that provides different notions of time according to the different levels of finality.
type Clock interface {
	// Accepted returns a notion of time that is anchored to the latest accepted block.
	Accepted() RelativeTime

	Confirmed() RelativeTime

	// Snapshot returns a snapshot of all time values tracked in the clock read atomically.
	Snapshot() *Snapshot

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}

// Snapshot contains the snapshot of all time values tracked in the clock.
type Snapshot struct {
	AcceptedTime          time.Time
	RelativeAcceptedTime  time.Time
	ConfirmedTime         time.Time
	RelativeConfirmedTime time.Time
}
