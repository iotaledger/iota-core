package poa

import (
	"time"

	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

// WithActivityWindow sets the duration for which a validator is recognized as active after issuing a block.
func WithActivityWindow(activityWindow time.Duration) options.Option[SeatManager] {
	return func(p *SeatManager) {
		p.optsActivityWindow = activityWindow
	}
}

func WithOnlineCommitteeStartup(optsOnlineCommittee ...iotago.AccountID) options.Option[SeatManager] {
	return func(p *SeatManager) {
		p.optsOnlineCommitteeStartup = optsOnlineCommittee
	}
}
