package chainmanager

import (
	"github.com/iotaledger/hive.go/runtime/event"
	iotago "github.com/iotaledger/iota.go/v4"
)

type Events struct {
	CommitmentPublished *event.Event1[*ChainCommitment]
	CommitmentBelowRoot *event.Event1[iotago.CommitmentID]
	ForkDetected        *event.Event1[*Fork]

	RequestCommitment *event.Event1[iotago.CommitmentID]

	event.Group[Events, *Events]
}

var NewEvents = event.CreateGroupConstructor(func() *Events {
	return &Events{
		CommitmentPublished: event.New1[*ChainCommitment](),
		CommitmentBelowRoot: event.New1[iotago.CommitmentID](),
		ForkDetected:        event.New1[*Fork](),
		RequestCommitment:   event.New1[iotago.CommitmentID](),
	}
})
