package chainmanager

import (
	"github.com/iotaledger/hive.go/runtime/event"
	"github.com/iotaledger/iota-core/pkg/protocol/engine/eviction"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	CommitmentMissing         *event.Event1[iotago.CommitmentID]
	MissingCommitmentReceived *event.Event1[iotago.CommitmentID]
	CommitmentBelowRoot       *event.Event1[iotago.CommitmentID]
	ForkDetected              *event.Event1[*Fork]
	EvictionState             *eviction.Events

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		CommitmentMissing:         event.New1[iotago.CommitmentID](),
		MissingCommitmentReceived: event.New1[iotago.CommitmentID](),
		CommitmentBelowRoot:       event.New1[iotago.CommitmentID](),
		ForkDetected:              event.New1[*Fork](),
	}
})
