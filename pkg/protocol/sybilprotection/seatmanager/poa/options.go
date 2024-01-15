package poa

import (
	"github.com/iotaledger/hive.go/runtime/options"
	iotago "github.com/iotaledger/iota.go/v4"
)

func WithOnlineCommitteeStartup(optsOnlineCommittee ...iotago.AccountID) options.Option[SeatManager] {
	return func(p *SeatManager) {
		p.optsOnlineCommitteeStartup = optsOnlineCommittee
	}
}
