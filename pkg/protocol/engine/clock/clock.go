package clock

import (
	"github.com/iotaledger/hive.go/runtime/module"
)

// Clock is an engine module that provides different notions of time according to the different levels of finality.
type Clock interface {
	// PreAccepted returns a notion of time that is anchored to the latest preAccepted block.
	PreAccepted() RelativeTime

	// Accepted returns a notion of time that is anchored to the latest accepted block.
	Accepted() RelativeTime

	// PreConfirmed returns a notion of time that is anchored to the latest preConfirmed block.
	PreConfirmed() RelativeTime

	Confirmed() RelativeTime

	// Interface embeds the required methods of the module.Interface.
	module.Interface
}
